package postgres

import (
	"rugram/internal/domain"
)

func hasUnreadChannelReactionPG(reactions domain.ChannelMessageReactions) bool {
	for _, recent := range reactions.Recent {
		if recent.Unread {
			return true
		}
	}
	return false
}
