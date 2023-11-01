package lsp

var langs = map[string]Language{
	"go": Language{
		Command: "gopls",
		Args:    []string{"serve"},
		Ft:      "go",
	},
}

type Language struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Ft      string
}

func GetLanguage(ft string) (Language, bool) {
	l, ok := langs[ft]
	return l, ok
}
