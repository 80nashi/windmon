package windmon

import (
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
  
  "bytes"
  "fmt"
  "image/png"
  "image/jpeg"
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
  MICS_SHIMODA_IMG_URL = "http://www6.kaiho.mlit.go.jp/map/kisho_genkyo/03kanku_shimoda.gif" // this is really a png. wtf
  MICS_SUKA_URL = "http://www6.kaiho.mlit.go.jp/03kanku/yokosuka/kisyou.html"
  
  MICS_TABLE_XPATH = "//node()[@id='kishoInfo']//thead"
  MICS_DATE_XPATH = "//node()[@id='kisyouDate']"
)

type WindData struct {
  Date string
  Table string
  Img []byte
}

// http://www.jma.go.jp/jp/amedas_h/today-46211.html
// http://www6.kaiho.mlit.go.jp/03kanku/shimoda/ shift-jis
// http://www6.kaiho.mlit.go.jp/03kanku/yokosuka/kisyou.html
func getWindData(c appengine.Context) (*WindData, error) {
  windData := &WindData{}
  client := urlfetch.Client(c)
  resp, err := client.Get(MICS_URL)
  if err != nil {
    return nil, fmt.Errorf("could not get %s: %v", MICS_URL, err)
  }
  if resp.StatusCode != 200 {
    return nil, fmt.Errorf("server responded non-200: %s, %s", MICS_URL, resp.Status)
  }
  
  defer resp.Body.Close()
  // http://stackoverflow.com/questions/24101721/parse-broken-html-with-golang
  buf := new(bytes.Buffer)
  buf.ReadFrom(resp.Body)
  content := mahonia.NewDecoder("cp932").ConvertString(buf.String())
  
  doc, err := xhtml.Parse(strings.NewReader(content))
  // https://godoc.org/golang.org/x/net/html
  if err != nil {
    return nil, fmt.Errorf("could not parse HTML for %s: %v", MICS_URL, err)
  }
  var b bytes.Buffer
  xhtml.Render(&b, doc)
  fixed := strings.NewReader(b.String())
  
  root, err := xmlpath.ParseHTML(fixed)
  if err != nil {
    return nil, fmt.Errorf("could not parse HTML: %s\n Error: %v", content, err)
  }
  
  path := xmlpath.MustCompile(MICS_TABLE_XPATH)
  table, ok := path.String(root)
  if !ok {
    return nil, fmt.Errorf("could not find table path")
  }
  c.Infof("read table %s", table)
  windData.Table = table
  
  path = xmlpath.MustCompile(MICS_DATE_XPATH)
  date, ok := path.String(root)
  if !ok {
    return nil, fmt.Errorf("could not find date")
  }
  windData.Date = date
  
  imgResp, err := client.Get(MICS_SHIMODA_IMG_URL)
  if err != nil {
    return nil, fmt.Errorf("unable to get img from %s: %v", MICS_SHIMODA_IMG_URL, err)
  }
  if imgResp.StatusCode != 200 {
    return nil, fmt.Errorf("img server responded non-200: %s, %s", MICS_SHIMODA_IMG_URL, imgResp.Status)
  }
  defer imgResp.Body.Close()
  
  // XXX need to resize the image for Gratina2
  // JPG is more available: http://media.kddi.com/app/publish/torisetsu/pdf/gratina2_torisetsu_shousai.pdf
  // go image packages
  // image/gif, image/jpeg: http://golang.org/pkg/image/gif/#Encode
  
  pngImg, err := png.Decode(imgResp.Body)
  if err != nil {
    // we can do with only text info
    c.Infof("No image attached. Could not decode png: %v", err)
    return windData, nil
  }
  buf.Reset()
  err = jpeg.Encode(buf, pngImg, &jpeg.Options{Quality: 75})
  if err != nil {
    // we can do with text info only
    c.Infof("No image attached. Could not encode to jpeg: %v", err)
    return windData, nil
  }
  windData.Img = buf.Bytes()
  return windData, nil
}

func sendUpdate(addr string, w *WindData, c appengine.Context) error {
  msg := &gaeMail.Message{
    Sender: "news@windmon-miura.appspotmail.com",
    To: []string{addr},
    Subject: "Wind news " + w.Date,
    Body: w.Table,
    Attachments: []gaeMail.Attachment{{
                   Name: "img.jpg",
                   Data: w.Img,
                   ContentID: "<windmon-miura-img>",
                 },},
  }
  return gaeMail.Send(c, msg)
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
  
  wind, err := getWindData(c)
  if err != nil {
    c.Errorf("Could not get wind data: %v", err)
    return
  }
  for _, u := range userStatus {
    c.Infof("Sending update to %s. msg = %s / %s", u.Email, wind.Date, wind.Table)
    if err := sendUpdate(u.Email, wind, c); err != nil {
      c.Errorf("Unable to send email to %s: %v", u.Email, err)
    }
  }
}