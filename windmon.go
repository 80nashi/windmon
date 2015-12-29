package windmon

import (
  "encoding/base64"
  "fmt"
  "math/rand"
  "net/http"
  "net/mail"
  "strings"
  "time"
  

  "appengine"
  "appengine/datastore"
  gaeMail "appengine/mail"
  
  regist "gaelib/regist"
)


const (
  CONFIRM_MODEL = "confirm"
  USER_MODEL = "userstatus"
)

type Confirmation struct {
  Email string
  Code string
  Retry int
}

type UserStatus struct {
  Email string
  Status string
}

var myRegister = regist.MailHandler{processEmail}

/*
Borrowing registerDemo() pattern from google go api client libray.
https://github.com/google/google-api-go-client/blob/master/examples/main.go
https://github.com/google/google-api-go-client/blob/master/examples/bigquery.go

- Each data source registers its collector and alerter URL paths as well as
  GAE request handler for the paths in its init().
  I.e.;
    func init() {
      ds := generateDataSource("this_ds")
      registerSource(ds)
      http.HandleFunc(ds.CollectorUrl, thisDsCollectHandler)
      http.HandleFunc(ds.AlerterUrl, thisDsAlertHandler)
    }
- Each data source's collector or alerter URL is called via GAE TaskQueue,
  ensuring paralle execution of multiple data sources.
- i.e.; /collect -> windmon.go -> taskqueue -> /collect_this_ds
  (alerters work the same way through /alert)
  
Todo:
- Is there a way for windmon.go to register all collectors and alerters
  on behalf of each data source's inint()?
  GAE does not have func main() to be executed after all init() is called.
*/

var dataSource = make(map[string]DataSource)
type DataSource struct {
  CollectorUrl string
  AlerterUrl string
}

func registerSource(name string, ds DataSource) {
  if _, ok := dataSource[name]; ok {
    panic(name + " already registered")
  }
  dataSource[name] = ds
}

func generateDataSource(name string) DataSource {
  return DataSource {
    CollectorUrl: "/ds/" + name + "/collect",
    AlerterUrl: "/ds/" + name + "/alert",
  }
}

func getSubCode() string {
  r := rand.New(rand.NewSource(time.Now().UnixNano()))
  b := make([]byte, 32)
  for i := 0; i < 32; i++ {
    b[i] = byte(r.Int31())
  }
  return base64.StdEncoding.EncodeToString(b)
}

func unregisterUser(addr string, c appengine.Context) {
  q := datastore.NewQuery(USER_MODEL).
         Filter("Email =", addr).
         KeysOnly()
  keys, err := q.GetAll(c, nil)
  if err != nil {
    c.Errorf("Cound not query the model for %s: %v", addr, err)
    return
  }
  if len(keys) == 0 {
    c.Infof("No such user to unregister: %s", addr)
    return
  }
  for i := range keys {
    datastore.Delete(c, keys[i])
  }
  c.Infof("Removed user %s", addr)
  
  msg := &gaeMail.Message{
    Sender: "unregister@windmon-miura.appspotmail.com",
    To: []string{addr},
    Subject: "Email unregistered",
    Body: "user " + addr + " has been unregistered",
  }
  gaeMail.Send(c, msg)
}

func confirmSubscription(addr, code string, c appengine.Context) {
  // check if code is in the datastore
  q := datastore.NewQuery(CONFIRM_MODEL).
         Filter("Email =", addr).
         Filter("Code =", code).
         KeysOnly()
  keys, err := q.GetAll(c, nil)
  if err != nil {
    c.Errorf("Couldn't query the model for %s, %s: %v", addr, code, err)
    return
  }
  if len(keys) == 0 {
    c.Infof("No such confirmation code %s, %s", addr, code)
    return
  }
  
  // Now the user is confirmed.
  
  // Removing all pending requests for the user including stale codes
  q = datastore.NewQuery(CONFIRM_MODEL).
        Filter("Email =", addr).
        KeysOnly()
  keys, err = q.GetAll(c, nil)
  for i := range keys {
    datastore.Delete(c, keys[i])
    c.Infof("Pending confirmation codes were removed for %s, %s", addr, code)
  }
  
  msg := &gaeMail.Message{
    Sender: "confirm@windmon-miura.appspotmail.com",
    To: []string{addr},
    Subject: "Email subscribed",
    Body: "user " + addr + " has been subscribed",
  }
  gaeMail.Send(c, msg)
  
  userStatus := UserStatus{
    Email: addr,
    Status: "off",
  }
  _, err = datastore.Put(c,
                          datastore.NewIncompleteKey(c, USER_MODEL, nil),
                          &userStatus)
  if err != nil {
    c.Errorf("Couldn't create user status entry  %s: %v", addr, err)
    return
  }
  c.Infof("Created a user entry successfully for %s", addr)
}

func sendSubscription(addr string, c appengine.Context) {
  code := getSubCode()
  msg := &gaeMail.Message{
    Sender: "subscribe@windmon-miura.appspotmail.com",
    To: []string{addr},
    Subject: "confirm " + code,
    Body: "Reply without changing subject",
  }
  if err := gaeMail.Send(c, msg); err != nil {
    c.Errorf("Couldn't send email to %s for %s: %v", addr, code, err)
  }
  
  // XXXX if successful, register the code as (email, code, 0 (retry)) tuple.
  confirmation := Confirmation{
    Email: addr,
    Code: code,
    Retry: 0,
  }
  
  _, err := datastore.Put(c,
                            datastore.NewIncompleteKey(c, CONFIRM_MODEL, nil),
                            &confirmation)
  if err != nil {
    c.Errorf("Couldn't write confirmation code for %s, %s: %v", addr, code, err)
    return
  }
  c.Infof("Wrote confirmation successfully for %s, %s", addr, code)
}

func processEmail(msg *mail.Message, c appengine.Context) {
  c.Debugf("Yay, my own handler!  email from %v", msg.Header)
  
  // parse from address. return if error
  addr, err := mail.ParseAddress(msg.Header["From"][0])
  if err != nil {
    c.Errorf("Wrong email from: %s", msg.Header["From"][0])
    return
  }
  subject := msg.Header["Subject"][0]
  // if subscribe request starting with "reg", send subscription code and return
  if strings.HasPrefix(subject, "reg") {
    sendSubscription(addr.Address, c)
    return
  }
  // or unregsiter if "unreg"
  if strings.HasPrefix(subject, "unreg") {
    unregisterUser(addr.Address, c)
    return
  }
  
  // if confirmation, fully regist it and return
  pos := strings.Index(subject, "confirm ")
  if pos >= 0 {
    code := subject[pos + len("confirm "):]
    confirmSubscription(addr.Address, code, c)
    return
  }
  
  // check if email is there. return if none (not registered)
  q := datastore.NewQuery(USER_MODEL).
         Filter("Email =", addr.Address)
  var u []UserStatus
  keys, err := q.GetAll(c, &u)
  if err != nil {
    c.Errorf("Could not retrieve user status for %s: %v", addr.Address, err)
    return
  }
  if len(u) != 1 {
    c.Errorf("There's no such user %s, len(u) == %d", addr.Address, len(u))
    return
  }
  
  // If the subject is on/On/oN/ON, it's on.  Off otherwise
  if strings.EqualFold(subject, "on") {
    u[0].Status = "on"
  } else {
    u[0].Status = "off"
  }
  if _, err = datastore.Put(c, keys[0], &u[0]); err != nil {
    c.Errorf("Could not write new status for %s: %v", addr.Address, err)
  }
  c.Infof("Updated %s to %s", addr.Address, u[0].Status)
  return
}

func listDSHandler(w http.ResponseWriter, r *http.Request) {
  for key, ds := range dataSource {
    fmt.Fprintf(w, "data source: %s %+v\n", key, ds)
  }
}

func init() {
  // https://cloud.google.com/appengine/docs/go/mail/?hl=ja
  // http://golang.org/pkg/net/mail/
  http.HandleFunc("/_ah/mail/", myRegister.Handler)

  // crons
  http.HandleFunc("/update_wind", updateWind)
  
  http.HandleFunc("/", handler)
  http.HandleFunc("/listDataSources", listDSHandler)
  
}

func handler(w http.ResponseWriter, r *http.Request) {
  fmt.Fprint(w, "Hello, world!")
}