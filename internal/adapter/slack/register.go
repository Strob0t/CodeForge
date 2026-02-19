package slack

import "github.com/Strob0t/CodeForge/internal/port/notifier"

func init() {
	notifier.Register(providerName, func(config map[string]string) (notifier.Notifier, error) {
		return NewNotifier(config["webhook_url"]), nil
	})
}
