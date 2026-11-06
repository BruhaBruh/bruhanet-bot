package routes

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
)

var moscow = content.Moscow()

func formatTime(t time.Time) string { return content.FormatTime(t) }

func formatBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d Б", n)
	}
	units := []string{"КБ", "МБ", "ГБ", "ТБ", "ПБ"}
	v := float64(n) / 1024
	i := 0
	for ; i < len(units)-1 && v >= 1024; i++ {
		v /= 1024
	}
	return fmt.Sprintf("%.2f %s", v, units[i])
}

func pluralMonths(n int) string {
	return plural(n, "месяц", "месяца", "месяцев")
}

func plural(n int, one, few, many string) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return one
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return few
	}
	return many
}

// advanceText — длина аванса для текста: «1 день», «36 часов».
func advanceText(hours int) string {
	if hours%24 == 0 {
		return fmt.Sprintf("%d %s", hours/24, plural(hours/24, "день", "дня", "дней"))
	}
	return fmt.Sprintf("%d %s", hours, plural(hours, "час", "часа", "часов"))
}

// tariffVars — переменные months («3 месяца») и price для текстов о тарифе.
func tariffVars(month, price int) map[string]string {
	return map[string]string{"months": fmt.Sprintf("%d %s", month, pluralMonths(month)), "price": strconv.Itoa(price)}
}
