package lsp

type Language struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}
