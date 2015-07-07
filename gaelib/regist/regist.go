// https://groups.google.com/forum/#!msg/golang-nuts/coEvrWIJGTs/75GzcefKVcIJ
package regist

import (
  "appengine"
  "net/http"
  "net/mail"
)

type MailHandler struct {
  Process func(*mail.Message, appengine.Context)
}

func (m MailHandler) Handler(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  defer r.Body.Close()
  msg, err := mail.ReadMessage(r.Body)
  if err != nil {
    c.Errorf("Error reading body: %v", err)
    return
  }
  if m.Process != nil {
    m.Process(msg, c)
  } else {
    defaultProcessMail(msg, c)
  }
}

func defaultProcessMail(msg *mail.Message, c appengine.Context) {
  c.Infof("Received email from %v", msg.Header)
}