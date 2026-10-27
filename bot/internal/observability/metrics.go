// Package observability — метрики, readiness и HTTP-эндпоинты бота.
package observability

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registry — единый реестр бота; метрики объявлены здесь, остальные пакеты только инкрементят.
var Registry = prometheus.NewRegistry()

var (
	Updates = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "bot_updates_total", Help: "Processed Telegram updates.",
	}, []string{"route", "result"})
	UpdateDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "bot_update_duration_seconds", Help: "Update handling time.", Buckets: prometheus.DefBuckets,
	}, []string{"route"})
	ExternalRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "bot_external_requests_total", Help: "Calls to external services.",
	}, []string{"service", "operation", "result"})
	ExternalDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "bot_external_request_duration_seconds", Help: "External call time.", Buckets: prometheus.DefBuckets,
	}, []string{"service", "operation"})
	Payments = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "bot_payments_total", Help: "Payment flow events.",
	}, []string{"stage", "months"})
	PaymentsConfirmedRubles = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "bot_payments_confirmed_rubles_total", Help: "Confirmed payments amount in rubles.",
	}, []string{"months"})
	RemindersSent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "bot_reminders_sent_total", Help: "Expiry reminders sent.",
	}, []string{"kind"})
	Broadcasts = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "bot_broadcasts_total", Help: "Admin broadcasts started.",
	})
	BroadcastMessages = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "bot_broadcast_messages_total", Help: "Broadcast messages by delivery result.",
	}, []string{"result"})
	Ready = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "bot_ready", Help: "1 if all dependencies are up.",
	})
	DependencyUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bot_dependency_up", Help: "1 if the dependency check passes.",
	}, []string{"dependency"})
)

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		Updates, UpdateDuration, ExternalRequests, ExternalDuration,
		Payments, PaymentsConfirmedRubles, RemindersSent, Broadcasts, BroadcastMessages,
		Ready, DependencyUp,
	)
}

// InitBusinessSeries заранее создаёт серии бизнес-счётчиков нулём: иначе increase() теряет
// первое событие серии после каждого рестарта.
func InitBusinessSeries(months []int) {
	for _, m := range months {
		ms := strconv.Itoa(m)
		for _, stage := range []string{"started", "photo", "confirmed", "declined"} {
			Payments.WithLabelValues(stage, ms)
		}
		PaymentsConfirmedRubles.WithLabelValues(ms)
	}
	for _, k := range []string{"week", "day"} {
		RemindersSent.WithLabelValues(k)
	}
	for _, r := range []string{"sent", "failed"} {
		BroadcastMessages.WithLabelValues(r)
	}
}
