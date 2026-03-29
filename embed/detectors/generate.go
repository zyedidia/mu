//go:build ignore

package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"

	"github.com/zyedidia/ftdetect"
)

var in = flag.String("in", ".", "input directory")
var out = flag.String("out", "detectors.dat", "output data file")

func main() {
	flag.Parse()

	files, err := ioutil.ReadDir(*in)
	if err != nil {
		log.Fatal(err)
	}

	ds := make(ftdetect.Detectors)
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		data, err := ioutil.ReadFile(filepath.Join(*in, f.Name()))
		if err != nil {
			log.Printf("%s: read: %v\n", f.Name(), err)
			continue
		}

		var d ftdetect.Detector
		err = json.Unmarshal(data, &d)
		if err != nil {
			log.Printf("%s: unmarshal: %v\n", f.Name(), err)
			continue
		}

		ds.RegisterDetector(&d)
	}

	data, err := ds.Serialize()
	if err != nil {
		log.Fatal(err)
	}

	ioutil.WriteFile(*out, data, 0666)
}
