package service

import (
	"encoding/json"
	"go-finance/internal/model"
	"net/http"
	"time"
)

const OunceToTael = 1.20565

// GetMetalPrices fetches external APIs
func GetMetalPrices() (model.ExchangeRates, error) {
	rates := model.ExchangeRates{UsdVND: 25000.0, BtcVND: 2400000000.0}

	client := http.Client{Timeout: 5 * time.Second}

	// 1. USD Rate
	if resp, err := client.Get("https://open.er-api.com/v6/latest/USD"); err == nil {
		defer resp.Body.Close()
		var d struct {
			Rates map[string]float64 `json:"rates"`
		}
		if json.NewDecoder(resp.Body).Decode(&d) == nil {
			rates.UsdVND = d.Rates["VND"]
		}
	}

	// 2. Gold World
	if resp, err := client.Get("https://api.gold-api.com/price/XAU"); err == nil {
		defer resp.Body.Close()
		var d struct {
			Price float64 `json:"price"`
		}
		if json.NewDecoder(resp.Body).Decode(&d) == nil {
			rates.GoldUSD = d.Price
		}
	}

	// 3. Silver World
	if resp, err := client.Get("https://api.gold-api.com/price/XAG"); err == nil {
		defer resp.Body.Close()
		var d struct {
			Price float64 `json:"price"`
		}
		if json.NewDecoder(resp.Body).Decode(&d) == nil {
			rates.SilverUSD = d.Price
			// Estimate VN Silver
			baseVND := rates.SilverUSD * rates.UsdVND * OunceToTael
			rates.VnSilver = baseVND * 1.05
		}
	}

	// 4. VN SJC
	if resp, err := client.Get("https://www.vang.today/api/prices?type=SJL1L10"); err == nil {
		defer resp.Body.Close()
		var d struct {
			Sell float64 `json:"sell"`
		}
		if json.NewDecoder(resp.Body).Decode(&d) == nil {
			rates.VnSJC = d.Sell
		}
	}

	// 5. [CẬP NHẬT] Bitcoin Rate (CoinGecko Direct VND)
	if resp, err := client.Get("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=vnd"); err == nil {
		defer resp.Body.Close()

		// Định nghĩa struct khớp với JSON của CoinGecko
		var d struct {
			Bitcoin struct {
				VND float64 `json:"vnd"`
			} `json:"bitcoin"`
		}

		if json.NewDecoder(resp.Body).Decode(&d) == nil {
			// Gán trực tiếp giá VND lấy được từ API
			rates.BtcVND = d.Bitcoin.VND
		}
	}

	return rates, nil
}
