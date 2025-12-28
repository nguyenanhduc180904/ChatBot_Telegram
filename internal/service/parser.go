package service

import (
	"go-finance/internal/model"
	"regexp"
	"strconv"
	"strings"
)

// ParseTransactionText xử lý tin nhắn và trả về danh sách các giao dịch
// Hỗ trợ cú pháp nhiều lệnh trên 1 dòng, ngăn cách bởi dấu phẩy hoặc xuống dòng
// Ví dụ: "chi 3k trà đá, +1m lương" -> 2 giao dịch
func ParseTransactionText(text string) ([]model.TransactionCreate, error) {
	var results []model.TransactionCreate

	// Regex pattern:
	// Group 1: Keywords (thu, chi, tk, tiết kiệm...)
	// Group 2: Signs (+, -)
	// Group 3: Amount (số + k/m), THÊM [-]? ĐỂ BẮT SỐ ÂM
	// Group 4: Unit (usd, $, btc, chỉ vàng...)
	// Group 5: Note (chuỗi còn lại cho đến khi gặp dấu phẩy hoặc xuống dòng)
	// CẬP NHẬT: Thêm [-]? vào trước [\d.,]+ để bắt được trường hợp số âm (ví dụ: -50k)
	pattern := `(?i)(?:(thu|chi|tk|tiết\s?kiệm|tiet\s?kiem)|([+\-]))\s*([-]?[\d.,]+[km]?)\s*(usd|\$|btc|bitcoin|chỉ\s?vàng)?\s*([^,\n]*)`
	re := regexp.MustCompile(pattern)

	// FindAllStringSubmatch tìm tất cả các vị trí khớp trong chuỗi
	matches := re.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		// match[0] là toàn bộ chuỗi khớp
		kwStr := match[1]
		signStr := match[2]
		amountStr := match[3]
		unitStr := match[4]
		noteStr := match[5]

		// --- 1. Xác định Type ---
		var transType string
		kwLower := strings.ToLower(kwStr)
		if kwLower != "" {
			if strings.Contains(kwLower, "tk") || strings.Contains(kwLower, "tiết kiệm") {
				transType = "tiet_kiem"
			} else {
				transType = kwLower // thu, chi
			}
		} else {
			// Dùng dấu +/-
			if signStr == "+" {
				transType = "thu"
			} else {
				transType = "chi"
			}
		}

		// --- 2. Xử lý Amount ---
		multiplier := 1.0
		amountClean := strings.ToLower(strings.TrimSpace(amountStr))

		// Xử lý suffix k, m
		if strings.HasSuffix(amountClean, "k") {
			multiplier = 1000
			amountClean = amountClean[:len(amountClean)-1]
		} else if strings.HasSuffix(amountClean, "m") {
			multiplier = 1000000
			amountClean = amountClean[:len(amountClean)-1]
		}

		// Thay thế dấu phẩy bằng dấu chấm để parse float
		amountClean = strings.ReplaceAll(amountClean, ",", ".")
		val, err := strconv.ParseFloat(amountClean, 64)
		if err != nil {
			continue
		}
		val = val * multiplier

		// Check số âm hoặc bằng 0 -> Bỏ qua (Đây là chỗ sẽ fix được test case)
		if val <= 0 {
			continue
		}

		// --- 3. Xác định Currency ---
		currency := "VND"
		u := strings.ToLower(unitStr)
		if u == "usd" || u == "$" {
			currency = "USD"
		} else if u == "btc" || u == "bitcoin" {
			currency = "BTC"
		} else if strings.Contains(u, "vàng") {
			currency = "GOLD"
		}

		// --- 4. Xử lý Note và Validate ---
		finalNote := strings.TrimSpace(noteStr)

		// Rule 1: Tiết kiệm KHÔNG được có note
		if transType == "tiet_kiem" {
			if finalNote != "" {
				continue
			}
		} else {
			// Rule 2: Thu/Chi BẮT BUỘC có note và CHỈ dùng VND
			if finalNote == "" {
				continue
			}
			if currency != "VND" {
				continue
			}
		}

		// --- 5. Tự động phân loại (Category) ---
		category := ""
		if transType == "chi" {
			category = CategorizeExpense(finalNote)
		}

		results = append(results, model.TransactionCreate{
			Type:     transType,
			Amount:   val,
			Note:     finalNote,
			Currency: currency,
			Category: category,
		})
	}

	return results, nil
}
