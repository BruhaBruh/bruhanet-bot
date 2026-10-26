package routes

import (
	"fmt"
	"time"
)

var monthNames = [...]string{
	"", "января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

var moscow = loadMoscow()

func loadMoscow() *time.Location {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return time.FixedZone("MSK", 3*60*60)
	}
	return loc
}

func formatTime(t time.Time) string {
	t = t.In(moscow)
	zone, _ := t.Zone()
	return fmt.Sprintf("%d %s %d г. в %02d:%02d, %s",
		t.Day(), monthNames[t.Month()], t.Year(), t.Hour(), t.Minute(), zone)
}

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
	switch {
	case n%10 == 1 && n%100 != 11:
		return "месяц"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return "месяца"
	}
	return "месяцев"
}

func tariffTitle(month, price int) string {
	return fmt.Sprintf("%d %s за %d ₽", month, pluralMonths(month), price)
}
