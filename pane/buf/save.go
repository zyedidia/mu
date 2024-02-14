package buf

import (
	"errors"
	"os"

	"github.com/zyedidia/mu/pkg/output"
)

func (bp *BufPane) Save(args []string) error {
	if len(args) >= 1 {
		return bp.SaveAs(args[0])
	}

	if !bp.HasOutput() {
		path, canceled := bp.messager.Prompt("save", "Filename: ")
		if canceled {
			return errors.New("save failed: no output file")
		}
		return bp.SaveAs(path)
	}
	err := bp.Buffer.Save()
	if errors.Is(err, os.ErrPermission) {
		if ok, err := bp.saveWithSudo(); ok {
			return err
		}
	}
	return err
}

func (bp *BufPane) SaveAs(path string) error {
	bp.SetOutput(&output.File{
		Path: path,
	})
	err := bp.Buffer.Save()
	if errors.Is(err, os.ErrPermission) {
		if ok, err := bp.saveWithSudo(); ok {
			return err
		}
	}
	return err
}

func (bp *BufPane) saveWithSudo() (bool, error) {
	if f := bp.FileOutput(); f != nil && output.HasRootFile {
		choice, cancel := bp.messager.CharPrompt("File cannot be written, save with sudo? (y,n,esc)")
		if cancel {
			return false, nil
		}
		if choice == "y" {
			suspend, resume := bp.Editor.SuspendResume()
			bp.SetOutput(&output.RootFile{
				Suspend: suspend,
				Resume:  resume,
				RootCmd: "sudo",
				Path:    f.Path,
			})
			return true, bp.Save(nil)
		}
	}
	return false, nil
}

func (bp *BufPane) CheckModified() error {
	if bp.ExtModified() {
		choice, cancel := bp.messager.CharPrompt("The file being edited has been externally modified. Reload from disk? (y,n,esc)")
		if choice == "y" && !cancel {
			return bp.Reload()
		}
		bp.SetExtModified()
	}
	return nil
}
