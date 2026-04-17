package telegram

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

type catalog map[string]string

//go:embed locales/ru.yaml
var ruYAML []byte

var activeCatalog catalog

func init() {
	var ru catalog
	if err := yaml.Unmarshal(ruYAML, &ru); err != nil {
		panic(fmt.Sprintf("failed to load ru catalog: %v", err))
	}
	activeCatalog = ru
}

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
