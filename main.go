package main

import (
	"fmt"
	"log"
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
	goodSpreadString = "0.08"
	goodBidString = "0.25"
	goodAskString = "0.75"
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
	bestBid *decimal.Decimal
	bestAsk *decimal.Decimal
	spread *decimal.Decimal
	change float64
}

func main() {
	loadConfiguration()
	runScreener()
}

func loadConfiguration() {
	configuration = commons.LoadConfiguration(configurationPath, configuration)
}

func runScreener() {
	markets := []gamma.Market{}
	for _, symbol := range configuration.Symbols {
		now := time.Now()
		lowerSymbol := strings.ToLower(symbol.Symbol)
		month := strings.ToLower(now.Month().String())
		slug := fmt.Sprintf("%s-up-or-down-on-%s-%02d-%d", lowerSymbol, month, now.Day(), now.Year())
		market, err := gamma.GetMarket(slug)
		if err != nil {
			log.Fatalf("Failed to retrieve market: %v", err)
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
				bestBid := getOrderSummary(message.Bids)
				bestAsk := getOrderSummary(message.Asks)
				var spread *decimal.Decimal
				if bestBid != nil && bestAsk != nil {
					spreadValue := bestAsk.Sub(*bestBid)
					spread = &spreadValue
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
					bestBid: bestBid,
					bestAsk: bestAsk,
					spread: spread,
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
	goodSpread := mustParseDecimal(goodSpreadString)
	goodBid := mustParseDecimal(goodBidString)
	goodAsk := mustParseDecimal(goodAskString)
	header := []string{
		"Symbol",
		"Best Bid",
		"Best Ask",
		"Spread",
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
		bidString := getDecimalString(data.bestBid)
		if data.change < 0.0 && data.bestBid != nil && data.bestBid.GreaterThanOrEqual(goodBid) {
			bidString = green(bidString)
		}
		askString := getDecimalString(data.bestAsk)
		if data.change > 0.0 && data.bestAsk != nil && data.bestAsk.LessThanOrEqual(goodAsk) {
			askString = green(askString)
		}
		spreadString := getDecimalString(data.spread)
		if enableSpreadColors && data.spread != nil && data.spread.LessThanOrEqual(goodSpread) {
			spreadString = green(spreadString)
		}
		changeString := fmt.Sprintf("%+.2f%%", data.change)
		if data.change >= 0 {
			changeString = green(changeString)
		} else {
			changeString = red(changeString)
		}
		row := []string{
			data.symbol,
			bidString,
			askString,
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