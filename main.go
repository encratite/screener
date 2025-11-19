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
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/shopspring/decimal"
)

const (
	configurationPath = "configuration/configuration.yaml"
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
		// fmt.Printf("%s: bestBid = %.2f, bestAsk = %.2f, spread = %.2f\n", symbol.Symbol, market.BestBid, market.BestAsk, market.Spread)
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

func printTable(symbols []symbolData) {
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
		row := []string{
			data.symbol,
			getDecimalString(data.bestBid),
			getDecimalString(data.bestAsk),
			getDecimalString(data.spread),
			fmt.Sprintf("%+.2f%%", data.change),
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
}