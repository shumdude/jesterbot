package telegram

import "fmt"

type catalog map[string]string

var activeCatalog = ruCatalog

func tr(key string, args ...any) string {
	template, ok := activeCatalog[key]
	if !ok {
		panic(fmt.Sprintf("missing translation key: %s", key))
	}
	if len(args) == 0 {
		return template
	}
	return fmt.Sprintf(template, args...)
}
