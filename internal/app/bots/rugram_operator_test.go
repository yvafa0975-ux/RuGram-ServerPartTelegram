package bots

import (
	"context"
	"math"
	"testing"
	"time"

	adminapp "rugram/internal/admin"
	"rugram/internal/domain"
	"rugram/internal/store/memory"
)

type testRuGramFreeze struct {
	current domain.AccountFreeze
	calls   int
}

func (f *testRuGramFreeze) AccountFreeze(_ context.Context, id int64) (domain.AccountFreeze, bool, error) {
	if f.current.UserID != id {
		return domain.AccountFreeze{}, false, nil
	}
	return f.current, true, nil
}

func (f *testRuGramFreeze) SetAccountFrozen(_ context.Context, req adminapp.SetAccountFrozenRequest) (adminapp.CommandResult, error) {
	f.calls++
	f.current = domain.AccountFreeze{UserID: req.UserID, Frozen: req.Frozen, Since: time.Now().UTC(),
		Until: req.Until, AppealURL: req.AppealURL, Actor: req.Actor, Reason: req.Reason, CommandID: req.CommandID}
	if !req.Frozen {
		f.current.Since = time.Time{}
	}
	return adminapp.CommandResult{}, nil
}

func TestRuGramOperatorBotChecksIDAndButton(t *testing.T) {
	ctx := context.Background()
	users := memory.NewUserStore()
	operator, err := users.Create(ctx, domain.User{Username: "paradox", FirstName: "Paradox"})
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := users.Create(ctx, domain.User{Username: "outsider", FirstName: "Outsider"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := users.Create(ctx, domain.User{Username: "target", FirstName: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	freezes := &testRuGramFreeze{}
	service := NewService(users, memory.NewBotStore(users), memory.NewMessageStore(memory.NewDialogStore()),
		WithRuGramOperatorBot([]int64{operator.ID}, freezes), WithPublicBaseURL("https://rugram.net"))
	if !service.HandlesBot(rugramOperatorBotUserID) || service.rugramOperators[outsider.ID] {
		t.Fatal("operator bot access list is wrong")
	}
	reply, err := service.rugramOperatorAccountReply(ctx, operator.ID, target)
	if err != nil || reply.ReplyMarkup == nil || len(reply.ReplyMarkup.Inline) != 1 {
		t.Fatalf("operator reply: %+v err=%v", reply, err)
	}
	button := reply.ReplyMarkup.Inline[0][0].Data
	denied, handled, err := service.OnCallbackQuery(ctx, domain.BotCallbackQuery{BotUserID: rugramOperatorBotUserID, UserID: outsider.ID, Data: button})
	if err != nil || !handled || !denied.Alert || freezes.calls != 0 {
		t.Fatalf("unauthorized callback: answer=%+v handled=%v calls=%d err=%v", denied, handled, freezes.calls, err)
	}
	query := domain.BotCallbackQuery{ID: 1, BotUserID: rugramOperatorBotUserID, UserID: operator.ID, Data: button}
	_, handled, err = service.OnCallbackQuery(ctx, query)
	if err != nil || !handled || freezes.calls != 1 || !freezes.current.Frozen || freezes.current.Until.Unix() != math.MaxInt32 {
		t.Fatalf("freeze callback: handled=%v calls=%d state=%+v err=%v", handled, freezes.calls, freezes.current, err)
	}
	_, _, err = service.OnCallbackQuery(ctx, query)
	if err != nil || freezes.calls != 1 {
		t.Fatalf("replayed button changed account again: calls=%d err=%v", freezes.calls, err)
	}
	unfreezeReply, err := service.rugramOperatorAccountReply(ctx, operator.ID, target)
	if err != nil || unfreezeReply.ReplyMarkup == nil {
		t.Fatalf("unfreeze button: err=%v", err)
	}
	unfreezeQuery := domain.BotCallbackQuery{ID: 2, BotUserID: rugramOperatorBotUserID, UserID: operator.ID,
		Data: unfreezeReply.ReplyMarkup.Inline[0][0].Data}
	_, handled, err = service.OnCallbackQuery(ctx, unfreezeQuery)
	if err != nil || !handled || freezes.calls != 2 || freezes.current.Frozen {
		t.Fatalf("unfreeze callback: handled=%v calls=%d state=%+v err=%v", handled, freezes.calls, freezes.current, err)
	}
}
