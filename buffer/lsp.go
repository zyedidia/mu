package buffer

import (
	"log"
)

func (b *BufferData) LoadLsp() {
	if b.Options["filetype"] == nil {
		return
	}
	if b.Len() < lspCutoff {
		var err error
		b.Lsp, err = b.LspManager.Open(b.Options["filetype"].(string), b.in.FullName(), string(b.Bytes()), b.lspVersion)
		if err != nil {
			log.Println("lsp error:", err)
		}
	}
}
