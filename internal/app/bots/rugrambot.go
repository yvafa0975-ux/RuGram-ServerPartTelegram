package bots

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	adminapp "rugram/internal/admin"
	"rugram/internal/domain"
)

const (
	rugramCallbackPrefix = "gs:account:"
	// The account-freeze schema stores frozen_until as an int32 timestamp. The
	// maximum value gives the operator bot an effectively indefinite freeze while
	// staying inside the same validation rules as the admin API.
	rugramFreezeUntilUnix int64 = 1<<31 - 1
)

// rugramFreezeAdmin is the narrow admin surface the built-in bot needs. The
// command still goes through admin.Service, so audit rows, idempotency,
// projection invalidation and online freeze notifications remain identical to
// actions from the admin API.
type rugramFreezeAdmin interface {
	AccountFreeze(context.Context, int64) (domain.AccountFreeze, bool, error)
	SetAccountFrozen(context.Context, adminapp.SetAccountFrozenRequest) (adminapp.CommandResult, error)
}

// WithRuGramAdmin enables the built-in @rugram moderation bot and installs
// its numeric private-chat allowlist. Empty IDs leave the bot discoverable but
// deny all moderation actions.
func WithRuGramAdmin(admin rugramFreezeAdmin, chatIDs []int64) Option {
	return func(s *Service) {
		s.rugramAdmin = admin
		s.rugramAdminChatIDs = make(map[int64]struct{}, len(chatIDs))
		for _, id := range chatIDs {
			if id > 0 {
				s.rugramAdminChatIDs[id] = struct{}{}
			}
		}
	}
}

func (s *Service) rugramChatAllowed(chatID int64) bool {
	if s == nil || chatID <= 0 {
		return false
	}
	_, ok := s.rugramAdminChatIDs[chatID]
	return ok
}

func (s *Service) respondAsRuGram(userID int64, body string) {
	mu := s.serviceBotReplyLock(domain.RuGramBotUserID, userID)
	mu.Lock()
	defer mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var reply botReply
	if !s.rugramChatAllowed(userID) {
		reply = botReply{Text: "Нет доступа: этот chat_id не входит в allowlist RuGram."}
	} else {
		reply = s.handleRuGramCommand(ctx, userID, body)
	}
	s.sendServiceBotReply(ctx, domain.RuGramBotUserID, userID, reply)
}

func rugramHelpText() string {
	return "Команды @rugram:\n\n" +
		"/account @username — показать статус и кнопку переключения\n" +
		"/freeze @username — подготовить заморозку\n" +
		"/unfreeze @username — подготовить разморозку\n" +
		"/help — эта справка"
}

func parseRuGramCommand(body string) (command, argument string) {
	fields := strings.Fields(strings.TrimSpace(body))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return "", ""
	}
	command = strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	if len(fields) > 1 {
		argument = fields[1]
	}
	return command, argument
}

func (s *Service) handleRuGramCommand(ctx context.Context, _ int64, body string) botReply {
	command, argument := parseRuGramCommand(body)
	switch command {
	case "start", "help":
		return botReply{Text: rugramHelpText()}
	case "account", "status":
		if argument == "" {
			return botReply{Text: "Укажи аккаунт: /account @username или /account 123456789."}
		}
		return s.rugramAccountReply(ctx, argument, nil)
	case "freeze":
		if argument == "" {
			return botReply{Text: "Укажи аккаунт: /freeze @username или /freeze 123456789."}
		}
		desired := true
		return s.rugramAccountReply(ctx, argument, &desired)
	case "unfreeze":
		if argument == "" {
			return botReply{Text: "Укажи аккаунт: /unfreeze @username или /unfreeze 123456789."}
		}
		desired := false
		return s.rugramAccountReply(ctx, argument, &desired)
	default:
		return botReply{Text: rugramHelpText()}
	}
}

func (s *Service) rugramResolveTarget(ctx context.Context, reference string) (domain.User, error) {
	if s == nil || s.users == nil {
		return domain.User{}, fmt.Errorf("user lookup is unavailable")
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return domain.User{}, fmt.Errorf("account is required")
	}
	if strings.HasPrefix(reference, "@") {
		reference = strings.TrimPrefix(reference, "@")
	}
	if reference == "" {
		return domain.User{}, fmt.Errorf("username is empty")
	}
	if id, err := strconv.ParseInt(reference, 10, 64); err == nil {
		user, found, lookupErr := s.users.ByID(ctx, id)
		if lookupErr != nil {
			return domain.User{}, lookupErr
		}
		if !found {
			return domain.User{}, fmt.Errorf("аккаунт %q не найден", reference)
		}
		return user, nil
	}
	user, found, err := s.users.ByUsername(ctx, reference)
	if err != nil {
		return domain.User{}, err
	}
	if !found {
		return domain.User{}, fmt.Errorf("аккаунт @%s не найден", reference)
	}
	return user, nil
}

func rugramUserLabel(user domain.User) string {
	if user.Username != "" {
		return "@" + user.Username
	}
	return fmt.Sprintf("id=%d", user.ID)
}

func (s *Service) rugramAccountReply(ctx context.Context, reference string, requested *bool) botReply {
	user, err := s.rugramResolveTarget(ctx, reference)
	if err != nil {
		return botReply{Text: err.Error()}
	}
	if domain.IsSystemUserID(user.ID) {
		return botReply{Text: "Системные аккаунты через @rugram менять нельзя."}
	}
	if s.rugramAdmin == nil {
		return botReply{Text: "Модерация через @rugram не настроена."}
	}
	freeze, found, err := s.rugramAdmin.AccountFreeze(ctx, user.ID)
	if err != nil {
		return botReply{Text: "Не удалось прочитать состояние аккаунта."}
	}
	current := found && freeze.Frozen
	desired := !current
	if requested != nil {
		desired = *requested
	}
	state := "разморожен"
	if current {
		state = "заморожен"
	}
	action := "Заморозить"
	style := domain.MarkupButtonStyleDanger
	if !desired {
		action = "Разморозить"
		style = domain.MarkupButtonStyleSuccess
	}
	text := fmt.Sprintf("Аккаунт %s (id %d) сейчас %s.", rugramUserLabel(user), user.ID, state)
	text += "\nНажми одну кнопку для изменения состояния."
	return botReply{
		Text: text,
		ReplyMarkup: &domain.MessageReplyMarkup{
			Type: domain.MessageReplyMarkupInline,
			Inline: [][]domain.MarkupButton{{{
				Type:  domain.MarkupButtonCallback,
				Text:  action,
				Style: style,
				Data:  []byte(fmt.Sprintf("%s%d:%d", rugramCallbackPrefix, user.ID, boolInt(desired))),
			}}},
		},
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseRuGramCallback(data []byte) (targetID int64, frozen bool, ok bool) {
	parts := strings.Split(string(data), ":")
	if len(parts) != 4 || parts[0] != "gs" || parts[1] != "account" {
		return 0, false, false
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || id <= 0 {
		return 0, false, false
	}
	state, err := strconv.Atoi(parts[3])
	if err != nil || (state != 0 && state != 1) {
		return 0, false, false
	}
	return id, state == 1, true
}

func (s *Service) onRuGramCallback(ctx context.Context, query domain.BotCallbackQuery) (domain.BotCallbackAnswer, bool, error) {
	if query.Peer.Type != domain.PeerTypeUser || query.Peer.ID != query.UserID || !s.rugramChatAllowed(query.UserID) {
		return domain.BotCallbackAnswer{Message: "Нет доступа для этого chat_id.", Alert: true}, true, nil
	}
	targetID, desired, ok := parseRuGramCallback(query.Data)
	if !ok {
		return domain.BotCallbackAnswer{Message: "Кнопка устарела.", Alert: true}, true, nil
	}
	if domain.IsSystemUserID(targetID) {
		return domain.BotCallbackAnswer{Message: "Системные аккаунты менять нельзя.", Alert: true}, true, nil
	}
	if s.rugramAdmin == nil || s.users == nil {
		return domain.BotCallbackAnswer{Message: "Модерация недоступна.", Alert: true}, true, nil
	}
	target, found, err := s.users.ByID(ctx, targetID)
	if err != nil {
		return domain.BotCallbackAnswer{}, true, err
	}
	if !found {
		return domain.BotCallbackAnswer{Message: "Аккаунт уже не существует.", Alert: true}, true, nil
	}

	// Serialize clicks for one target across all three operator chats, then
	// re-read the state so a stale button cannot overwrite a newer action.
	mu := s.serviceBotReplyLock(domain.RuGramBotUserID, targetID)
	mu.Lock()
	defer mu.Unlock()
	currentFreeze, currentFound, err := s.rugramAdmin.AccountFreeze(ctx, targetID)
	if err != nil {
		return domain.BotCallbackAnswer{}, true, err
	}
	current := currentFound && currentFreeze.Frozen
	if current == desired {
		return domain.BotCallbackAnswer{Message: "Состояние уже изменено."}, true, nil
	}
	commandID := fmt.Sprintf("rugram-%d-%d-%d-%d", time.Now().UnixNano(), query.UserID, targetID, boolInt(desired))
	request := adminapp.SetAccountFrozenRequest{
		CommandMeta: adminapp.CommandMeta{
			CommandID: commandID,
			Actor:     fmt.Sprintf("rugram-bot:chat:%d", query.UserID),
			Reason:    "inline moderation in built-in @rugram",
		},
		UserID: targetID,
		Frozen: desired,
	}
	if desired {
		request.Until = time.Unix(rugramFreezeUntilUnix, 0).UTC()
		request.AppealURL = "https://t.me/SpamBot"
	}
	if _, err := s.rugramAdmin.SetAccountFrozen(ctx, request); err != nil {
		return domain.BotCallbackAnswer{Message: "Изменение не применено.", Alert: true}, true, err
	}

	action := "разморожен"
	if desired {
		action = "заморожен"
	}
	notification := fmt.Sprintf("@rugram: аккаунт %s (id %d) %s.\nИнициатор: chat_id %d.", rugramUserLabel(target), target.ID, action, query.UserID)
	s.notifyRuGramOperators(notification)
	// Leave the initiator with a fresh one-button card whose action is the
	// inverse of the committed state. The old card remains immutable in the
	// message history and becomes harmlessly idempotent.
	cardCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	updatedCard := s.rugramAccountReply(cardCtx, strconv.FormatInt(targetID, 10), nil)
	s.sendServiceBotReply(cardCtx, domain.RuGramBotUserID, query.UserID, updatedCard)
	return domain.BotCallbackAnswer{Message: fmt.Sprintf("Аккаунт %s.", action)}, true, nil
}

func (s *Service) notifyRuGramOperators(text string) {
	if s == nil || s.messages == nil {
		return
	}
	ids := make([]int64, 0, len(s.rugramAdminChatIDs))
	for id := range s.rugramAdminChatIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, id := range ids {
		if _, sent := s.sendServiceBotReplyResult(ctx, domain.RuGramBotUserID, id, botReply{Text: text}); !sent {
			s.log.Warn("rugram bot: operator notification failed", zap.Int64("chat_id", id))
		}
	}
}
