package keyboard

import "github.com/go-telegram/bot/models"

func Row(buttons ...Button) []Button {
	return buttons
}

func Grid(n int, buttons ...Button) [][]Button {
	if n < 1 {
		n = 1
	}
	rows := make([][]Button, 0, (len(buttons)+n-1)/n)
	for i := 0; i < len(buttons); i += n {
		rows = append(rows, buttons[i:min(i+n, len(buttons))])
	}
	return rows
}

func Inline(rows ...[]Button) *models.InlineKeyboardMarkup {
	markup := make([][]models.InlineKeyboardButton, 0, len(rows))
	for _, row := range rows {
		out := make([]models.InlineKeyboardButton, 0, len(row))
		for _, b := range row {
			out = append(out, b.build())
		}
		markup = append(markup, out)
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: markup}
}
