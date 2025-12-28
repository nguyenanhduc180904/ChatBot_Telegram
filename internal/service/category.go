package service

import (
	"regexp"
	"strings"
)

var categoryKeywords = map[string][]string{
	"ăn uống": {
		"ăn", "ăn sáng", "ăn trưa", "ăn tối", "cafe", "cà phê", "trà đá", "chè", "nhậu",
		"bia", "đồ ăn", "đồ uống", "quán", "phở", "bún", "cơm", "trà sữa",
	},
	"sinh hoạt": {
		"sửa xe", "đổ xăng", "xăng", "tiền điện", "điện", "nước", "điện thoại",
		"học phí", "internet", "wifi", "gas", "rác", "phí", "bảo hiểm",
	},
	"hưởng thụ": {
		"spa", "du lịch", "massage", "cắt tóc", "làm tóc", "xem phim", "phim",
		"karaoke", "trò chơi", "game", "makeup",
	},
}

// Hàm phân loại chi tiêu
func CategorizeExpense(note string) string {
	text := strings.ToLower(note)
	if text == "" {
		return "khác"
	}

	for category, keywords := range categoryKeywords {
		for _, k := range keywords {
			// (^|[^\p{L}]) : Bắt đầu chuỗi HOẶC ký tự trước đó KHÔNG phải là chữ cái
			// ([^\p{L}]|$) : Ký tự tiếp theo KHÔNG phải là chữ cái HOẶC kết thúc chuỗi
			pattern := `(?i)(^|[^\p{L}])` + regexp.QuoteMeta(k) + `([^\p{L}]|$)`
			matched, _ := regexp.MatchString(pattern, text)
			if matched {
				return category
			}
		}
	}
	return "khác"
}
