package windmon
// https://cloud.google.com/appengine/docs/go/tools/localunittesting/

import (
  "fmt"
  "io/ioutil"
  "testing"
  
  "launchpad.net/xmlpath"
)

const (
  testHtmlFile = "titus_test.html"
)

func parseTestHtml(t *testing.T) (*xmlpath.Node, error) {
  dat, err := ioutil.ReadFile(testHtmlFile)
  if err != nil {
    return nil, fmt.Errorf("could not read %s: %v", testHtmlFile, err)
  }
  return reparseHtml(string(dat))
}

func TestGetTitusCsvLink(t *testing.T) {
  root, err := parseTestHtml(t)
  if err != nil {
    t.Errorf("could not parse test html: %v", err)
  }
  link, err := getTitusCsvLink(root)
  if err != nil {
    t.Errorf("could not get csv link: %v", err)
  }
  if link != "http://www6.kaiho.mlit.go.jp/tokyowan/cgi-local/titus/../../weather-data/days/2015/20151219.csv" {
    t.Errorf("link is not correct: %s", link)
  }
}

func TestConvertWindDirection(t *testing.T) {
  d, ok := convertWindDirection("NNE")
  if !ok || d != 22.5 {
    t.Errorf("conversion failed for NNE %s:%f", ok, d)
  }
  d, ok = convertWindDirection("hogehoge")
  if ok {
    t.Errorf("conversion should not be ok")
  }
}

func TestConvertTime(t *testing.T) {
  ts, err := convertTime("2015/12/19", "20:15")
  if err != nil {
    t.Errorf("could not parse time: %v", err)
  }
  if ts.Unix() != 1450523700 {
    t.Errorf("did not parse the time correctly: %d != %s",
        1450523700, ts.Unix())
  }
}

func TestConvertCsv(t *testing.T) {
  test_csv_mixed := `2015/12/19,12:45,NNW,7,N,12,NNE,10,1026,20000,NNE,8,NNE,13,NNE,6
2015/12/19,13:00,NNW,7,N,10,NNE,11,1026,20000,` // truncated on the second line
  
  tdata, _ := convertCsv(test_csv_mixed)
  if len(tdata) != 6 { // only the first line (6 entries) should be processed
    t.Errorf("conversion failed for test_csv_mixed: %d", len(tdata))
  }
  t.Logf("tdata: %+v", tdata)
}