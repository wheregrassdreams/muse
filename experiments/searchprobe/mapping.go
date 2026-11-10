package searchprobe

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/lang/cjk"
	"github.com/blevesearch/bleve/v2/mapping"
)

func schema() *mapping.IndexMappingImpl {
	m := bleve.NewIndexMapping()
	m.DefaultAnalyzer = cjk.AnalyzerName
	d := bleve.NewDocumentStaticMapping()
	for _, name := range []string{"Path", "Title", "Tags", "Aliases", "Body"} {
		f := bleve.NewTextFieldMapping()
		f.Analyzer = cjk.AnalyzerName
		d.AddFieldMappingsAt(name, f)
	}
	m.DefaultMapping = d
	return m
}
