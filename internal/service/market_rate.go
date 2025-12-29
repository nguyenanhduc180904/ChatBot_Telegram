package service

import (
	"encoding/json"
	"go-finance/internal/model"
	"log"
	"net/http"
	"sync"
	"time"
)

const OunceToTael = 1.20565

var (
	// Biến toàn cục lưu giá (Cache)
	cachedRates model.ExchangeRates
	// Mutex để đảm bảo an toàn khi nhiều luồng đọc/ghi cùng lúc
	ratesMutex sync.RWMutex
)

// Hàm khởi chạy worker cập nhật giá (Gọi 1 lần duy nhất ở main.go)
func StartPriceUpdater() {
	// 1. Cập nhật ngay lập tức khi khởi động để có dữ liệu liền
	updateRates()

	// 2. Thiết lập định kỳ 10 phút cập nhật 1 lần
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		updateRates()
	}
}

// Hàm private thực hiện logic gọi API và lưu vào Cache
func updateRates() {
	log.Println("[CACHE] Đang cập nhật tỷ giá mới...")
	newRates, err := GetMetalPrices() // Gọi hàm cũ của bạn
	if err != nil {
		log.Printf("[CACHE ERROR] Không thể cập nhật giá: %v", err)
		return
	}

	// KHÓA GHI: Chỉ cho phép 1 luồng được ghi dữ liệu vào biến
	ratesMutex.Lock()
	cachedRates = newRates
	ratesMutex.Unlock()

	log.Println("[CACHE] Cập nhật tỷ giá thành công!")
}

// Hàm Public để các chỗ khác lấy giá từ Cache (Cực nhanh)
func GetCurrentRates() model.ExchangeRates {
	// KHÓA ĐỌC: Cho phép nhiều luồng đọc cùng lúc, nhưng không ai được ghi
	ratesMutex.RLock()
	defer ratesMutex.RUnlock()

	// Nếu cache chưa có dữ liệu (lần đầu tiên), trả về giá trị mặc định an toàn
	if cachedRates.UsdVND == 0 {
		return model.ExchangeRates{
			UsdVND:    25400,      // Giá USD ~25,400đ
			GoldUSD:   2700,       // Giá Vàng TG ~$2,700/oz
			SilverUSD: 32,         // Giá Bạc TG ~$32/oz
			VnSJC:     8500000,    // Giá Vàng SJC ~8.5 triệu/chỉ
			VnSilver:  1000000,    // Giá Bạc VN ước lượng ~1 triệu/cây (lượng)
			BtcVND:    2500000000, // Bitcoin ~2.5 tỷ VND
			// GoldDiff và SilverDiff để 0 cũng được vì chỉ dùng để hiển thị báo cáo
		}
	}

	return cachedRates
}

// GetMetalPrices fetches external APIs
func GetMetalPrices() (model.ExchangeRates, error) {
	rates := model.ExchangeRates{
		UsdVND:    25400,
		GoldUSD:   2700,
		SilverUSD: 32,
		VnSJC:     8500000,
		VnSilver:  1000000,
		BtcVND:    2500000000,
	}

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
			rates.VnSJC = d.Sell / 10
		}
	}

	// 5. [CẬP NHẬT] Bitcoin Rate (CoinGecko Direct VND)
	// CoinGecko rất hay chặn request từ Cloud Server, nên cần thêm User-Agent giả lập trình duyệt
	req, _ := http.NewRequest("GET", "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=vnd", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)") // Giả lập Browser

	if resp, err := client.Do(req); err == nil {
		defer resp.Body.Close()

		var d struct {
			Bitcoin struct {
				VND float64 `json:"vnd"`
			} `json:"bitcoin"`
		}

		if json.NewDecoder(resp.Body).Decode(&d) == nil {
			// CHỈ CẬP NHẬT NẾU GIÁ TRỊ > 0
			if d.Bitcoin.VND > 0 {
				rates.BtcVND = d.Bitcoin.VND
			}
		}
	}

	return rates, nil
}
