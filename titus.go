// package has to be windmon because titus.go is in the same directory as
// windmon.go
// can't mv titus.go to titus/titus.go because it fails to load other libraries
// when deploying (gcloud preview app deploy) while goapp build works in
// titus/titus.go.  weird...
// Also, needs ln -s . src in ~/workspace to make gopath right for goapp,
// but need to remove it while gcloud deploy.
package windmon

/* 
- data location:
  http://www6.kaiho.mlit.go.jp/tokyowan/cgi-local/titus/weg.cgi
- The cgi returns a page which contains a link to the most recent csv
- data sample
in the order of 東京13号地	本牧	観音埼	剱埼灯台	洲埼灯台	伊豆大島・風早埼
2015/12/19,00:00,NNW,5,N,6,NNE,9,1025,25000,NNE,6,NNE,9,N,2

- link in the cgi output
<html>
<body>
<div id="page">
...
<td class="csv"><A href="../../weather-data/days/2015/20151219.csv">
- 観音埼 is special because it has additinal 気圧 and 視程 (meters) columns
*/

import (
  mahonia "code.google.com/p/mahonia"
  xhtml "golang.org/x/net/html"
  "launchpad.net/xmlpath"
  
  "bytes"
  "encoding/csv"
  "fmt"
  "io"
  "net/http"
  "strings"
  "strconv"
  "time"
  
  "appengine"
  "appengine/urlfetch"
)

const (
  titusDataSourceName = "titus"
  
  TitusCgiBaseUrl = "http://www6.kaiho.mlit.go.jp/tokyowan/cgi-local/titus"
  TitusCgiUrl = TitusCgiBaseUrl + "/weg.cgi"
  // http://www.w3schools.com/xsl/xpath_syntax.asp
  titusCsvXpath = "//td[@class='optcsv']/a/@href"
)

type TitusData struct {
  T int64 // time.Unix()
  Location string // 
  WindSpeed float64 // in m/s
  WindDirection float64 // N=0, NNE=30, NEN=60, E=90 and so on.
  
  // Pressure and Visibility is only avaialble for 観音崎
  Pressure float64 // in hPa
  Visibility float64 // in meters
}

func init() {
  ds := generateDataSource(titusDataSourceName)
  registerSource(titusDataSourceName, ds)
  http.HandleFunc(ds.CollectorUrl, collectHandlerDummy)
  http.HandleFunc(ds.AlerterUrl, collectHandlerDummy)
}

func reparseHtml(s string) (*xmlpath.Node, error) {
  content := mahonia.NewDecoder("cp932").ConvertString(s)
  
  doc, err := xhtml.Parse(strings.NewReader(content))
  if err != nil {
    return nil, fmt.Errorf("could not parse HTML for %s ...(snip): %v",
        content[:30], err)
  }
  
  var b bytes.Buffer
  xhtml.Render(&b, doc)
  fixed := strings.NewReader(b.String())
  
  root, err := xmlpath.ParseHTML(fixed)
  if err != nil {
    return nil, fmt.Errorf("could not rebuild HTML for %s ...(snip): %v",
        content[:30], err)
  }
  
  return root, nil
}

func getUrlBody(c appengine.Context, url string) (*bytes.Buffer, error) {
  client := urlfetch.Client(c)
  resp, err := client.Get(url)
  if err != nil {
    return nil, fmt.Errorf("failed to retrieve URL %s: %v", url, err)
  }
  
  if resp.StatusCode != 200 {
    return nil, fmt.Errorf("ul returned non-200 status: %s", resp.Status)
  }
  
  defer resp.Body.Close()
  buf := new(bytes.Buffer)
  buf.ReadFrom(resp.Body)
  
  return buf, nil
}

func getParsedRoot(c appengine.Context, url string) (*xmlpath.Node, error) {
  buf, err := getUrlBody(c, url)
  if err != nil {
    return nil, err
  }
  
  return reparseHtml(buf.String())
}

func getTitusCsvLink(root *xmlpath.Node) (string, error) {
  path := xmlpath.MustCompile(titusCsvXpath)
  csvLink, ok := path.String(root)
  if ok {
    return TitusCgiBaseUrl + "/" + csvLink, nil
  } else {
    return "", fmt.Errorf("could not find xpath %s", titusCsvXpath)
  }
}

func convertLocation(n int) (string, bool) {
  locationMap := map [int] string {
    0: "東京13号地",
    1: "本牧",
    2: "観音埼",
    3: "剱埼灯台",
    4: "洲埼灯台",
    5: "伊豆大島・風早埼",
  }
  
  v, ok := locationMap[n]
  return v, ok
}

func convertWindDirection(d string) (float64, bool) {
  directionMap := map [string] float64 {
    "N"  : 0,
    "NNE": 22.5,
    "NE" : 45,
    "ENE": 67.5,
    "E"  : 90,
    "ESE": 112.5,
    "SE" : 135,
    "SSE": 157.5,
    "S"  : 180,
    "SSW": 202.5,
    "SW" : 225,
    "WSW": 247.5,
    "W"  : 270,
    "WNW": 292.5,
    "NW" : 315,
    "NNW": 337.5,
  }
  
  v, ok := directionMap[d]
  return v, ok
}

func convertTime(d string, t string) (time.Time, error) {
  return time.Parse("2006/01/02 15:04 -0700", d + " " + t + " +0900")
}

func getRecord(locationId int, windDirection, windSpeed string) *TitusData {
  loc, ok1 := convertLocation(locationId)
  wd, ok2 := convertWindDirection(windDirection)
  ws, err1 := strconv.ParseFloat(windSpeed, 64)
  
  if ok1 && ok2 && err1 == nil {
    return &TitusData {
      Location: loc,
      WindSpeed: ws,
      WindDirection: wd,
    }
  }
  return nil
}

func getRecord4(locationId int, windDirection, windSpeed, pressure, visibility string) *TitusData {
  t := getRecord(locationId, windDirection, windSpeed)
  pr, err2 := strconv.ParseFloat(pressure, 64)
  vis, err3 := strconv.ParseFloat(visibility, 64)
  
  if t != nil && err2 == nil && err3 == nil {
    t.Pressure = pr
    t.Visibility = vis
    return t
  }
  return nil
}

func addRecord(d []TitusData, t int64, td *TitusData) []TitusData {
  if td == nil {
    return d
  }
  
  td.T = t
  return append(d, *td)
}

func convertCsv(in string) ([]TitusData, error) {
  r := csv.NewReader(strings.NewReader(in))
  d := []TitusData{}
  for {
    record, err := r.Read() // one record at a time
    if err == io.EOF {
      break // end of data
    }
    if err == nil && len(record) == 16 {
      t, err := convertTime(record[0], record[1])
      if err != nil {
        continue
      }
      ts := t.Unix()
      d = addRecord(d, ts, getRecord(0, record[2], record[3]))
      d = addRecord(d, ts, getRecord(1, record[4], record[5]))
      d = addRecord(d, ts, getRecord4(2, record[6], record[7], record[8], record[9]))
      d = addRecord(d, ts, getRecord(3, record[10], record[11]))
      d = addRecord(d, ts, getRecord(4, record[12], record[13]))
      d = addRecord(d, ts, getRecord(5, record[14], record[15]))
    }
    // ignore error or truncated records silently
  }
  return d, nil
}

func getTitusData(c appengine.Context) ([]TitusData, error) {
  if root, err := getParsedRoot(c, TitusCgiUrl); err != nil {
    return nil, err
  } else if csvLink, err := getTitusCsvLink(root); err != nil {
    return nil, err
  } else if buf, err := getUrlBody(c, csvLink); err != nil {
    return nil, err
  } else {
    return convertCsv(buf.String())
  }
}

func addOrUpdate(c appengine.Context, t TitusData) {
  return
}

func collectHandler(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  c.Infof("titus collect handler called")
  
  titusData, err := getTitusData(c)
  if err != nil {
    c.Warningf("could not get titus data: %v", err)
    return
  }
  
  c.Infof("successfully collected data %d", len(titusData))
  
  for _, t := range titusData {
    addOrUpdate(c, t)
  }
  
}

func collectHandlerDummy(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  c.Infof("titus collect handler (dummy) called")
}
