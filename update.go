package windmon

import (
  "bytes"
  // These do not work in GAE environment : https://github.com/oschwald/maxminddb-golang/issues/11
  // 
  // "code.google.com/p/go.text/encoding/japanese"
  // "code.google.com/p/go.text/transform"
  
  // go get golang.org/x/net/html
  // go get golang.org/x/text/encoding
  // then move x/net/html and x/text to workspace/ (same dir as app.yaml).
  // Do not include net/ipv4 and such that would lead to an error like:
  //  go-app-builder: Failed parsing input: parser: bad import "syscall" in src/golang.org/x/net/icmp/endpoint.go
  xhtml "golang.org/x/net/html"
  
  // https://godoc.org/launchpad.net/xmlpath#Path
  "launchpad.net/xmlpath"
  
  // Another possible issue is the use of unkeyed fieds.
  // would need to change golang.org/x/text/unicode/rangetable/rangetable.go to
  // 
	//		rt.R16 = append(rt.R16, unicode.Range16{Lo: uint16(r), Hi: uint16(r), Stride: 1})
	//	} else {
	//		rt.R32 = append(rt.R32, unicode.Range32{Lo: uint32(r), Hi: uint32(r), Stride: 1})
  //
  // by inserting key names (Lo, Hi and Stride).
  
  
  // http://kaorumori.hatenadiary.com/entry/2015/04/03/143231
  mahonia "code.google.com/p/mahonia"
  
  "fmt"
  "net/http"
  "strings"

  "appengine"
  "appengine/datastore"
  "appengine/urlfetch"
  gaeMail "appengine/mail"
)

const (
  MICS_URL = MICS_SHIMODA_URL
  MICS_SHIMODA_URL = "http://www6.kaiho.mlit.go.jp/03kanku/shimoda/"
  MICS_SHIMODA_IMG_URL = "http://www6.kaiho.mlit.go.jp/map/kisho_genkyo/03kanku_shimoda.gif"
  MICS_SUKA_URL = "http://www6.kaiho.mlit.go.jp/03kanku/yokosuka/kisyou.html"
  
  MICS_TABLE_XPATH = "//node()[@id='kishoInfo']//thead"
  MICS_DATE_XPATH = "//node()[@id='kisyouDate']"
)

// http://www.jma.go.jp/jp/amedas_h/today-46211.html
// http://www6.kaiho.mlit.go.jp/03kanku/shimoda/ shift-jis
// http://www6.kaiho.mlit.go.jp/03kanku/yokosuka/kisyou.html
func getWindData(c appengine.Context) (string, string) {
  client := urlfetch.Client(c)
  resp, err := client.Get(MICS_URL)
  if err != nil {
    c.Errorf("Could not get data from %s: %v", MICS_URL, err)
    return "N/A " + MICS_URL, ""
  }
  if resp.StatusCode != 200 {
    c.Infof("Could not get data from %s: %s", MICS_URL, resp.Status)
    return fmt.Sprintf("Status is %s", resp.Status), ""
  }
  
  defer resp.Body.Close()
  // http://stackoverflow.com/questions/24101721/parse-broken-html-with-golang
  buf := new(bytes.Buffer)
  buf.ReadFrom(resp.Body)
  content := mahonia.NewDecoder("cp932").ConvertString(buf.String())
  
  doc, err := xhtml.Parse(strings.NewReader(content))
  // https://godoc.org/golang.org/x/net/html
  if err != nil {
    c.Errorf("Could not parse HTML for %s: %v", MICS_URL, err)
    return "Could not parse HTML", ""
  }
  var b bytes.Buffer
  xhtml.Render(&b, doc)
  fixed := strings.NewReader(b.String())
  
  root, err := xmlpath.ParseHTML(fixed)
  if err != nil {
    c.Errorf("Could not parse HTML: %s\n Error: %v", content, err)
    return "ParseHTML NG", ""
  }
  
  path := xmlpath.MustCompile(MICS_TABLE_XPATH)
  table, ok := path.String(root)
  if !ok {
    return "NG", "NG"
  }
  c.Infof("read table %s", table)
  
  path = xmlpath.MustCompile(MICS_DATE_XPATH)
  date, ok := path.String(root)
  if !ok {
    return "NG", table
  }
  return date, table
}

func sendUpdate(addr, subj, body string, c appengine.Context) {
  msg := &gaeMail.Message{
    Sender: "news@windmon-miura.appspotmail.com",
    To: []string{addr},
    Subject: "Wind news " + subj,
    Body: body,
  }
  gaeMail.Send(c, msg)
}

func updateWind(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  c.Infof("Cron executed")
  
  q := datastore.NewQuery(USER_MODEL).
         Filter("Status =", "on")
  var userStatus []UserStatus
  if _, err := q.GetAll(c, &userStatus); err != nil {
    c.Errorf("Could not query active users, not sending update: %v", err)
    return
  }
  if len(userStatus) == 0 {
    c.Infof("No user to send update. Quitting")
    return
  }
  
  date, table := getWindData(c)
  for _, u := range userStatus {
    c.Infof("Sending update to %s. msg = %s / %s", u.Email, date, table)
    sendUpdate(u.Email, date, table, c)
  }
}