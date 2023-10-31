package buffer

import (
	"log"
	"os"

	"github.com/zyedidia/mu/lsp"
)

func (b *BufferData) LoadLsp() {
	if l, ok := lsp.GetLanguage(b.Options["filetype"].(string)); ok {
		server, err := lsp.StartServer(l)
		if err != nil {
			log.Println("error starting LSP", err)
		} else {
			wd, _ := os.Getwd()
			server.Initialize(wd)
			b.lsp = server
		}
	}
}
