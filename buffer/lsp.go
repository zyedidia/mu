package buffer

import (
	"log"
	"os"

	"github.com/zyedidia/mu/lsp"
)

func (b *BufferData) LoadLsp() {
	if b.Options["filetype"] == nil {
		return
	}
	if l, ok := lsp.GetLanguage(b.Options["filetype"].(string)); ok {
		server, err := lsp.StartServer(l)
		if err != nil {
			log.Println("error starting LSP", err)
		} else if b.Len() < lspCutoff {
			wd, _ := os.Getwd()
			server.Initialize(wd)
			b.Lsp = server

			b.Lsp.DidOpen(b.in.FullName(), l.Ft, string(b.Bytes()), b.lspVersion)
		}
	}
}
