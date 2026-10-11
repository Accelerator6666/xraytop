package doctor

import (
 "strings"
 "testing"
)
func TestReportAndFailureClassification(t *testing.T){
 r:=Report{}
 r.add("one","PASS","ok")
 r.add("two","WARN","not configured")
 r.add("three","FAIL","unreadable\nsecret line")
 if r.Failures()!=1 {t.Fatalf("failures = %d",r.Failures())}
 out:=r.String()
 if !strings.Contains(out,"[FAIL]") || strings.Contains(out,"\nsecret line") {t.Fatal(out)}
}
