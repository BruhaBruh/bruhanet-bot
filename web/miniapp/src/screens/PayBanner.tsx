// Оплата, продление, докупка устройств и подарки живут только в боте (ручной перевод на карту).
// Кнопки «Открыть бота» нет: внутри Telegram ссылка на тот же бот не срабатывает, а вернуться в чат можно закрыв кабинет.
export function PayBanner() {
  return (
    <section className="mx-4 mb-4 rounded-lg bg-accent-tint p-4">
      <p className="subheadline text-accent-strong">Оплатить и продлить можно только в боте. Закрой кабинет и выбери «Оплатить» в меню.</p>
    </section>
  )
}
