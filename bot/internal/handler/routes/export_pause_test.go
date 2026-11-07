package routes

import "time"

// SetDeviceLimitPause подменяет паузу между повторами записи лимита нового юзера (в тестах без ожидания);
// возвращает функцию, которая вернёт прежнюю.
func SetDeviceLimitPause(d time.Duration) (restore func()) {
	prev := deviceLimitPause
	deviceLimitPause = d
	return func() { deviceLimitPause = prev }
}
