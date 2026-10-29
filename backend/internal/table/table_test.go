package table

import (
	"archive/zip"
	"bytes"
	"reflect"
	"testing"
)

func xlsx(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, body := range files {
		f, _ := w.Create(name)
		_, _ = f.Write([]byte(body))
	}
	_ = w.Close()
	return b.Bytes()
}

func TestRead(t *testing.T) {
	book := xlsx(t, map[string]string{
		"xl/workbook.xml": `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Cenník" sheetId="1" r:id="rId3"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId3" Target="worksheets/data.xml"/></Relationships>`,
		"xl/sharedStrings.xml": `<sst><si><t>Kód</t></si><si><t>Názov</t></si><si><r><t>Kábel </t></r><r><t>CYKY</t></r></si></sst>`,
		"xl/worksheets/data.xml": `<worksheet><sheetData>
			<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="inlineStr"><is><t>Cena</t></is></c></row>
			<row r="2"><c r="A2"><v>101</v></c><c r="B2" t="s"><v>2</v></c><c r="D2" t="b"><v>1</v></c><c r="C2"><v>2.19</v></c></row>
			<row r="3"></row>
		</sheetData></worksheet>`,
	})
	tests := []struct {
		name string
		data []byte
		want [][]string
		err  bool
	}{
		{"CSV so ; a BOM", []byte("\xEF\xBB\xBFKód;Názov;Cena\r\nA1;\"Práca; servis\";12,50\r\n\r\n"),
			[][]string{{"Kód", "Názov", "Cena"}, {"A1", "Práca; servis", "12,50"}}, false},
		{"CSV s čiarkou", []byte("kod,nazov\nB2,Kábel\n"), [][]string{{"kod", "nazov"}, {"B2", "Kábel"}}, false},
		{"CSV tabulátor", []byte("a\tb\n1\t2"), [][]string{{"a", "b"}, {"1", "2"}}, false},
		{"Windows-1250", []byte("N\xe1zov;MJ\n\x8akatu\xbea;ks\n"), [][]string{{"Názov", "MJ"}, {"Škatuľa", "ks"}}, false},
		{"XLSX", book, [][]string{{"Kód", "Názov", "Cena"}, {"101", "Kábel CYKY", "2.19", "áno"}}, false},
		{"starý XLS", []byte{0xD0, 0xCF, 0x11, 0xE0, 1, 2}, nil, true},
		{"poškodený ZIP", []byte("PK\x03\x04nezmysel"), nil, true},
		{"binárny súbor", []byte("a;b\x00c"), nil, true},
	}
	for _, tt := range tests {
		got, err := Read(tt.data)
		if (err != nil) != tt.err {
			t.Errorf("%s: chyba %v", tt.name, err)
			continue
		}
		if !tt.err && !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: %q", tt.name, got)
		}
	}
	if colIndex("AB12") != 27 || colIndex("A1") != 0 {
		t.Error("colIndex")
	}
}
