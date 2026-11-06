package routes_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/payflow"
)

const referrerID int64 = 777

// refEnv: пригласивший уже есть в users и в панели.
func refEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.pay.SeedUser(referrerID)
	e.rw.AddUser(referrerID, now.Add(10*day))
	return e
}

func TestStartRefRecordedOnce(t *testing.T) {
	e := refEnv(t)
	e.process(t, tgtest.Message(userID, fmt.Sprintf("/start ref_%d", referrerID)))
	if got, ok := e.pay.ReferredBy(userID); !ok || got != referrerID {
		t.Fatalf("ReferredBy = %d, %v", got, ok)
	}
	greeting := e.last(t, "sendMessage").Params.Get("text")

	// второй /start с другим payload: пригласивший не меняется
	e.pay.SeedUser(888)
	e.process(t, tgtest.Message(userID, "/start ref_888"))
	if got, _ := e.pay.ReferredBy(userID); got != referrerID {
		t.Fatalf("пригласивший перезаписан: %d", got)
	}
	if text := e.last(t, "sendMessage").Params.Get("text"); text != greeting {
		t.Fatalf("приветствие отличается: %q vs %q", text, greeting)
	}
}

// Мусор, свой id и неизвестный пригласивший не записываются и не меняют приветствие.
func TestStartRefRejected(t *testing.T) {
	for name, payload := range map[string]string{
		"self":     fmt.Sprintf("ref_%d", userID),
		"unknown":  "ref_999999",
		"letters":  "ref_abc",
		"zero":     "ref_0",
		"negative": "ref_-5",
		"huge":     "ref_99999999999999999999999",
		"other":    "promo_abc",
		"bare":     "ref_",
	} {
		t.Run(name, func(t *testing.T) {
			e := refEnv(t)
			e.process(t, tgtest.Message(userID, "/start "+payload))
			if _, ok := e.pay.ReferredBy(userID); ok {
				t.Fatal("реферал записан")
			}
			if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "Здравствуйте") {
				t.Fatalf("не приветствие: %s", text)
			}
		})
	}
}

// Юзер, уже начавший диалог, по ссылке не записывается.
func TestStartRefIgnoredForExistingUser(t *testing.T) {
	e := refEnv(t)
	e.process(t, tgtest.Message(userID, "/start"))
	e.process(t, tgtest.Message(userID, fmt.Sprintf("/start ref_%d", referrerID)))
	if _, ok := e.pay.ReferredBy(userID); ok {
		t.Fatal("существующий юзер стал приглашённым")
	}
}

// Админ с «/start login» по-прежнему получает кнопку входа, ref не мешает.
func TestStartLoginStillWorks(t *testing.T) {
	e := refEnv(t)
	e.process(t, tgtest.Message(adminID, "/start login"))
	if !strings.Contains(e.last(t, "sendMessage").Params.Get("reply_markup"), "auth?token=") {
		t.Fatalf("нет кнопки входа: %s", e.last(t, "sendMessage").Params.Get("reply_markup"))
	}
}

// payRef: друг приходит по ссылке, покупает на 1 месяц и админ подтверждает.
func payRef(t *testing.T, e *env) {
	t.Helper()
	e.process(t, tgtest.Message(userID, fmt.Sprintf("/start ref_%d", referrerID)))
	e.process(t, tgtest.Callback(userID, "buy:1"))
	e.process(t, tgtest.Photo(userID))
	e.process(t, confirmPress(e.pending(t, userID).ID))
}

func TestReferralBonusBothSides(t *testing.T) {
	e := refEnv(t)
	payRef(t, e)

	// друг: срок = prev(now) + месяц + 7 дней, одной записью после аванса
	want := now.Add(payflow.Month + 7*day)
	if got := e.rw.ExpireAt(userID); !got.Equal(want) {
		t.Fatalf("срок друга %v, want %v", got, want)
	}
	// пригласивший: +7 дней от текущего срока
	if got, w := e.rw.ExpireAt(referrerID), now.Add(17*day); !got.Equal(w) {
		t.Fatalf("срок пригласившего %v, want %v", got, w)
	}
	var friend, ref int
	for _, m := range e.pay.Outbox() {
		switch m.TextKey {
		case "referral.friend_bonus":
			friend++
			if m.ChatID != userID || m.Days != 7 {
				t.Fatalf("friend notice %+v", m)
			}
		case "referral.referrer_bonus":
			ref++
			if m.ChatID != referrerID || m.Days != 7 {
				t.Fatalf("referrer notice %+v", m)
			}
		}
	}
	if friend != 1 || ref != 1 {
		t.Fatalf("уведомлений: друг %d, пригласивший %d", friend, ref)
	}
	if r := e.pay.Rewards()[userID]; !r.FriendRewarded || !r.ReferrerRewarded || r.Referrer != referrerID {
		t.Fatalf("reward %+v", r)
	}

	// вторая оплата друга награды не даёт
	before := e.rw.ExpireAt(referrerID)
	e.process(t, tgtest.Callback(userID, "buy:1"))
	e.process(t, tgtest.Photo(userID))
	e.process(t, confirmPress(e.pending(t, userID).ID))
	if !e.rw.ExpireAt(referrerID).Equal(before) {
		t.Fatal("вторая оплата продлила пригласившего")
	}
}

func TestReferralOffInSettings(t *testing.T) {
	e := refEnv(t)
	e.src.SetSettings(map[string][]byte{"referralDays": []byte(`0`)})
	payRef(t, e)
	if got := e.rw.ExpireAt(userID); !got.Equal(now.Add(payflow.Month)) {
		t.Fatalf("срок друга %v", got)
	}
	if !e.rw.ExpireAt(referrerID).Equal(now.Add(10 * day)) {
		t.Fatal("пригласивший получил дни при выключенной рефералке")
	}
	if len(e.pay.Rewards()) != 0 {
		t.Fatal("награда создана")
	}
}

func TestReferralDeclinedNoBonus(t *testing.T) {
	e := refEnv(t)
	e.process(t, tgtest.Message(userID, fmt.Sprintf("/start ref_%d", referrerID)))
	e.process(t, tgtest.Callback(userID, "buy:1"))
	e.process(t, tgtest.Photo(userID))
	e.process(t, declinePress(e.pending(t, userID).ID))
	if len(e.pay.Rewards()) != 0 || !e.rw.ExpireAt(referrerID).Equal(now.Add(10*day)) {
		t.Fatal("отклонённый платёж дал бонус")
	}
}

func TestReferralBlockedReferrerGetsNothingNow(t *testing.T) {
	e := refEnv(t)
	e.pay.SetBlocked(referrerID, true)
	payRef(t, e)
	if !e.rw.ExpireAt(referrerID).Equal(now.Add(10 * day)) {
		t.Fatal("заблокированный пригласивший получил дни")
	}
	if r := e.pay.Rewards()[userID]; r.ReferrerRewarded || !r.FriendRewarded {
		t.Fatalf("reward %+v", r)
	}
	if got := e.rw.ExpireAt(userID); !got.Equal(now.Add(payflow.Month + 7*day)) {
		t.Fatalf("друг не получил бонус: %v", got)
	}
}

func TestReferralScreen(t *testing.T) {
	e := refEnv(t)
	e.process(t, tgtest.Message(userID, "/start"))
	if !strings.Contains(e.last(t, "sendMessage").Params.Get("reply_markup"), "Пригласить друга") {
		t.Fatal("нет кнопки в меню")
	}
	e.process(t, tgtest.Callback(userID, "referral"))
	link := fmt.Sprintf("https://t.me/bruhanet_test_bot?start=ref_%d", userID)
	edited := e.last(t, "editMessageText")
	if text := edited.Params.Get("text"); !strings.Contains(text, link) || !strings.Contains(text, "<b>7</b>") {
		t.Fatalf("экран: %s", text)
	}
	var copied string
	for _, b := range keyboardButtons(t, edited.Params.Get("reply_markup")) {
		if b.CopyText != nil {
			copied = b.CopyText.Text
		}
	}
	if copied != link {
		t.Fatalf("копируется %q, want %q", copied, link)
	}
	// username закэширован: второй показ не ходит в getMe
	n := len(e.srv.Calls("getMe"))
	e.process(t, tgtest.Callback(userID, "referral"))
	if len(e.srv.Calls("getMe")) != n || n != 1 {
		t.Fatalf("getMe вызван %d раз", len(e.srv.Calls("getMe")))
	}
}

func TestReferralScreenOff(t *testing.T) {
	e := refEnv(t)
	e.src.SetSettings(map[string][]byte{"referralDays": []byte(`0`)})
	e.process(t, tgtest.Message(userID, "/start"))
	if strings.Contains(e.last(t, "sendMessage").Params.Get("reply_markup"), "Пригласить друга") {
		t.Fatal("кнопка при referralDays=0")
	}
	e.process(t, tgtest.Callback(userID, "referral"))
	if text := e.text(t); strings.Contains(text, "t.me") {
		t.Fatalf("ссылка при выключенной рефералке: %s", text)
	}
}
