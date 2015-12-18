// package has to be windmon because titus.go is in the same directory as
// windmon.go
// can't mv titus.go to titus/titus.go because it fails to load other libraries
// when deploying (gcloud preview app deploy) while goapp build works in
// titus/titus.go.  weird...
// Also, needs ln -s . src in ~/workspace to make gopath right for goapp,
// but need to remove it while gcloud deploy.
package windmon

/* 
http://www6.kaiho.mlit.go.jp/tokyowan/cgi-local/titus/weg.cgi
- data sample
2015年 12月 18日
日	時	東京13号地	本牧	観音埼	剱埼灯台	洲埼灯台	伊豆大島・風早埼
風向	風速	風向	風速	風向	風速	気圧	視程	風向	風速	風向	風速	風向	風速
12/18	00:00		4		8		15	1019	20000		6		15		8

- wind direction is a gif.  file names are like "../../img/news/NNW2.gif"
- 観音埼 is special because it has additinal 気圧 and 視程 (meters) columns
*/

import (
  mahonia "code.google.com/p/mahonia"
  xhtml "golang.org/x/net/html"
  "launchpad.net/xmlpath"
  
  "bytes"
  "fmt"
  "net/http"
  "strings"
  
  "appengine"
  "appengine/urlfetch"
)

const (
  TitusUrl = "http://www6.kaiho.mlit.go.jp/tokyowan/cgi-local/titus/weg.cgi"
)

type TitusData struct {
  T int64 // time.Unix()
  WindSpeed int // in m/s
  WindDirection int // N=0, NNE=30, NEN=60, E=90 and so on.
  Pressure int // in hPa
  Visibility int // in meters
}

func init() {
  http.HandleFunc("/collect", collectHandler)
}

func getParsedRoot(c appengine.Context) (*xmlpath.Node, error) {
  client := urlfetch.Client(c)
  resp, err := client.Get(TitusUrl)
  if err != nil {
    return nil, fmt.Errorf("urlfetch failed to retrieve titus %s: %v",
                          TitusUrl, err)
  }
  if resp.StatusCode != 200 {
    return nil, fmt.Errorf("titus server returned %s", resp.Status)
  }
  
  defer resp.Body.Close()
  buf := new(bytes.Buffer)
  buf.ReadFrom(resp.Body)
  content := mahonia.NewDecoder("cp932").ConvertString(buf.String())
  
  c.Debugf("read converted content: %s", content[:30])
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

func collectHandler(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  c.Infof("titus collect handler called")
  
  root, err := getParsedRoot(c)
  if err != nil {
    c.Errorf("could not collect data: %v", err)
    return
  }
  
  c.Infof("successfully collected data: %v", root)
}
