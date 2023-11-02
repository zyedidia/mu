package lsp

var langs = map[string]Language{
	"go": Language{
		Command: "gopls",
		Args:    []string{"serve"},
		Ft:      "go",
	},
	"c": Language{
		Command: "clangd",
		Args:    []string{},
		Ft:      "c",
	},
	"c++": Language{
		Command: "clangd",
		Args:    []string{},
		Ft:      "cpp",
	},
	"haskell": Language{
		Command: "hie",
		Args:    []string{"--lsp"},
		Ft:      "haskell",
	},
	"python": Language{
		Command: "pyls",
		Args:    []string{},
		Ft:      "python",
	},
	"verilog": Language{
		Command: "svls",
		Args:    []string{},
		Ft:      "verilog",
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
