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
	mediocrePriceString = "0.75"
	goodPriceString = "0.50"
	goodSpreadString = "0.5"
	enableSpreadColors = false
	enableYahoo = true
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
	dateString := flag.String("date", "", "Run screener for a specific date")
	flag.Parse()
	configuration = commons.LoadConfiguration[Configuration](configurationPath)
	var date *time.Time
	if *dateString != "" {
		dateValue := commons.MustParseTime(*dateString)
		date = &dateValue
	}
	runScreener(date)
}

func runScreener(date *time.Time) {
	markets := []gamma.Market{}
	if date == nil {
		dateValue := time.Now()
		date = &dateValue
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
				var change float64
				if enableYahoo {
					value, err := yahoo.GetChange(yahooSymbol)
					if err != nil {
						log.Fatalf("Failed to retrieve last close for %s: %v", symbol.Symbol, err)
					}
					change = value
				} else {
					change = math.NaN()
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
	goodPrice := mustParseDecimal(goodPriceString)
	mediocrePrice := mustParseDecimal(mediocrePriceString)
	goodSpread := mustParseDecimal(goodSpreadString)
	header := []string{
		"Symbol",
		"Yes Price",
		"No Price",
		"Spread",
		"Change",
	}
	rows := [][]string{}
	for _, data := range symbols {
		getDecimalString := func (d *decimal.Decimal) string {
			if d != nil {
				return d.StringFixed(2)
			} else {
				return "-"
			}
		}
		green := color.New(color.FgGreen).SprintFunc()
		yellow := color.New(color.FgYellow).SprintFunc()
		red := color.New(color.FgRed).SprintFunc()
		yesString := getDecimalString(data.yes)
		if data.change > 0.0 && data.yes != nil && data.yes.IsPositive() {
			if data.yes.LessThanOrEqual(goodPrice) {
				yesString = green(yesString)
			} else if data.yes.LessThanOrEqual(mediocrePrice) {
				yesString = yellow(yesString)
			}
		}
		noString := getDecimalString(data.no)
		if data.change < 0.0 && data.no != nil && data.no.IsPositive() {
			if data.no.LessThanOrEqual(goodPrice) {
				noString = green(noString)
			} else if data.no.LessThanOrEqual(mediocrePrice) {
				noString = yellow(noString)
			}
		}
		spreadString := "-"
		if data.yes != nil && data.no != nil {
			one := decimal.NewFromInt(1)
			spread := data.yes.Add(*data.no).Sub(one)
			spreadString = getDecimalString(&spread)
			if spread.GreaterThanOrEqual(goodSpread) {
				spreadString = yellow(spreadString)
			}
		}
		var changeString string
		if !math.IsNaN(data.change) && !math.IsInf(data.change, 1) && !math.IsInf(data.change, -1) {
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
			spreadString,
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