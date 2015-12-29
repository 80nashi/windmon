package windmon
// https://cloud.google.com/appengine/docs/go/tools/localunittesting/

import (
  "testing"
)

func TestRegisterSource(t *testing.T) {
  // need to clear dataSource to nulify titus.go's init and so on.
  dataSource = make(map[string]DataSource)
  testSource := DataSource{CollectorUrl:"/testCollect", AlerterUrl:"/testAlert"}
  registerSource("testSource", testSource)
  
  if len(dataSource) != 1 {
    t.Errorf("dataSource should have registered but %d != 1: %+v",
             len(dataSource), dataSource)
  } else if testDs, ok := dataSource["testSource"]; !ok {
    t.Errorf("testSource should have registered but nonexistent")
  } else if testDs.CollectorUrl != "/testCollect" ||
            testDs.AlerterUrl != "/testAlert" {
    t.Errorf("testSource's collector or alerter URL is wrong: %+v",
             testDs)
  }
}
