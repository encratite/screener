package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/encratite/commons"
	"github.com/encratite/gamma"
	"github.com/encratite/yahoo"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/shopspring/decimal"
)

const (
	configurationPath = "configuration/configuration.yaml"
	goodPriceMaxString = "0.75"
	enableSpreadColors = false
)

var configuration *Configuration

type Configuration struct {
	Symbols []ScreenerSymbol `yaml:"symbols"`
}

type ScreenerSymbol struct {
	Symbol string `yaml:"symbol"`
	Yahoo string `yaml:"yahoo"`
}

type symbolData struct {
	symbol string
	yahoo string
	yes *decimal.Decimal
	no *decimal.Decimal
	spread *decimal.Decimal
	change float64
}

func main() {
	tomorrow := flag.Bool("tomorrow", false, "Run screener for tomorrow's daily markets, for use after session close")
	flag.Parse()
	loadConfiguration()
	runScreener(*tomorrow)
}

func loadConfiguration() {
	configuration = commons.LoadConfiguration(configurationPath, configuration)
}

func runScreener(tomorrow bool) {
	markets := []gamma.Market{}
	date := time.Now()
	if tomorrow {
		date = date.AddDate(0, 0, 1)
	}
	for _, symbol := range configuration.Symbols {
		lowerSymbol := strings.ToLower(symbol.Symbol)
		month := strings.ToLower(date.Month().String())
		slug := fmt.Sprintf("%s-up-or-down-on-%s-%d-%d", lowerSymbol, month, date.Day(), date.Year())
		market, err := gamma.GetMarket(slug)
		if err != nil || market.Slug == "" {
			log.Fatalf("Failed to retrieve market %s for symbol %s", slug, symbol.Symbol)
		}
		markets = append(markets, market)
	}
	assetIDs := gamma.GetAssetIDs(markets)
	symbols := make([]symbolData, len(configuration.Symbols))
	symbolCount := 0
	gamma.SubscribeToMarkets(assetIDs,  func(message gamma.BookMessage) bool {
		if message.EventType == gamma.BookEvent {
			index := slices.Index(assetIDs, message.AssetID)
			if index >= 0 {
				symbol := configuration.Symbols[index]
				yes := getOrderSummary(message.Asks)
				no := getOrderSummary(message.Bids)
				if no != nil {
					*no = decimal.NewFromInt(1).Sub(*no)
				}
				yahooSymbol := symbol.Symbol
				if symbol.Yahoo != "" {
					yahooSymbol = symbol.Yahoo
				}
				change, err := yahoo.GetChange(yahooSymbol)
				if err != nil {
					log.Fatalf("Failed to retrieve last close for %s: %v", symbol.Symbol, err)
				}
				data := symbolData{
					symbol: symbol.Symbol,
					yahoo: symbol.Yahoo,
					yes: yes,
					no: no,
					change: change,
				}
				symbols[index] = data
				symbolCount++
			} else {
				fmt.Printf("Unknown asset ID: %s\n", message.AssetID)
			}
			return symbolCount < len(configuration.Symbols)
		} else {
			return false
		}
	})
	printTable(symbols)
}

func getOrderSummary(summary []gamma.OrderSummary) *decimal.Decimal {
	if len(summary) > 0 {
		priceString :=  summary[len(summary) - 1].Price
		price, err := decimal.NewFromString(priceString)
		if err != nil {
			log.Fatalf("Failed to parse price: %s", priceString)
		}
		return &price
	} else {
		return nil
	}
}

func mustParseDecimal(value string) decimal.Decimal {
	output, err := decimal.NewFromString(value)
	if err != nil {
		log.Fatalf("Failed to parse decimal: %v", err)
	}
	return output
}

func printTable(symbols []symbolData) {
	goodPriceMax := mustParseDecimal(goodPriceMaxString)
	header := []string{
		"Symbol",
		"Yes Price",
		"No Price",
		"Change",
	}
	rows := [][]string{}
	for _, data := range symbols {
		getDecimalString := func (d *decimal.Decimal) string {
			if d != nil {
				return d.StringFixed(2)
			} else {
				return "N/A"
			}
		}
		green := color.New(color.FgGreen).SprintFunc()
		red := color.New(color.FgRed).SprintFunc()
		yesString := getDecimalString(data.yes)
		if data.change > 0.0 && data.yes != nil && data.yes.LessThanOrEqual(goodPriceMax) {
			yesString = green(yesString)
		}
		noString := getDecimalString(data.no)
		if data.change < 0.0 && data.no != nil && data.no.LessThanOrEqual(goodPriceMax) {
			noString = green(noString)
		}
		var changeString string
		if !math.IsNaN(data.change) {
			changeString = fmt.Sprintf("%+.2f%%", data.change)
			if data.change >= 0 {
				changeString = green(changeString)
			} else {
				changeString = red(changeString)
			}
		} else {
			changeString = "-"
		}
		row := []string{
			data.symbol,
			yesString,
			noString,
			changeString,
		}
		rows = append(rows, row)
	}
	alignments := []tw.Align{
		tw.AlignDefault,
		tw.AlignRight,
		tw.AlignRight,
		tw.AlignRight,
		tw.AlignRight,
	}
	tableConfig := tablewriter.WithConfig(tablewriter.Config{
		Header: tw.CellConfig{
			Formatting: tw.CellFormatting{AutoFormat: tw.Off},
			Alignment: tw.CellAlignment{Global: tw.AlignLeft},
		}},
	)
	fmt.Printf("\n")
	alignmentConfig := tablewriter.WithAlignment(alignments)
	table := tablewriter.NewTable(os.Stdout, tableConfig, alignmentConfig)
	table.Header(header)
	table.Bulk(rows)
	table.Render()
	fmt.Printf("\n")
}