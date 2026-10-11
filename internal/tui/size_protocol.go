package tui

import (
 "strconv"
 "strings"
)

// CSI 18 t (DECRQSS-compatible xterm window-size request) may return
// ESC [ 8 ; rows ; cols t. Do not assume that every terminal supports it.
// Decoder is shared with the keyboard reader so a reply cannot trigger page 8
// or type characters into search fields.
type sizeReplyDecoder struct { pending []byte }

// Feed emits normal key bytes (possibly including ESC) or a validated size.
// Incomplete prefixes remain buffered until Flush, which callers schedule.
func (d *sizeReplyDecoder) Feed(b byte) (keys []byte,cols,rows int,matched bool) {
 d.pending=append(d.pending,b)
 p:=d.pending
 const prefix="\x1b[8;"
 if len(p)>25 {return d.Flush(),0,0,false}
 if !strings.HasPrefix(prefix,string(p)) && !strings.HasPrefix(string(p),prefix) {
   return d.Flush(),0,0,false
 }
 if len(p)<=len(prefix){return nil,0,0,false}
 tail:=string(p[len(prefix):])
 // Valid prefix of digits;digits;t. Unknown CSI goes to normal key handler.
 semicolon:=strings.IndexByte(tail,';')
 if semicolon<0 {
   if len(tail)>4||!asciiDigits(tail) {return d.Flush(),0,0,false}
   return nil,0,0,false
 }
 left,right:=tail[:semicolon],tail[semicolon+1:]
 if len(left)==0||len(left)>3||!asciiDigits(left) {
   return d.Flush(),0,0,false
 }
 if strings.HasSuffix(right,"t") {
    digits:=strings.TrimSuffix(right,"t")
    if len(digits)==0||len(digits)>3||!asciiDigits(digits) {
       return d.Flush(),0,0,false
    }
    rows,_=strconv.Atoi(left)
    cols,_=strconv.Atoi(digits)
    d.pending=nil
    if cols<40||cols>180||rows<13||rows>200 {
       // Bounded valid reply, but unreasonable geometry: silently ignore it.
       return nil,0,0,false
    }
    return nil,cols,rows,true
 }
 if len(right)>3||!asciiDigits(right){return d.Flush(),0,0,false}
 return nil,0,0,false
}
func asciiDigits(s string)bool {
 for i:=0;i<len(s);i++ {if s[i]<'0'||s[i]>'9'{return false}}
 return true
}
func(d *sizeReplyDecoder) Flush()[]byte {
 b:=d.pending
 d.pending=nil
 return b
}
func(d *sizeReplyDecoder) Pending()bool{return len(d.pending)!=0}
