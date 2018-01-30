package proxy_if

import (
  "errors"
  "fmt"
  "regexp"
  "strings"
)

const (
  ProxyIfType_Baggage string = "baggage"
  ProxyIfType_Tag string = "tag"
)

type ProxyIf struct {
  input string
  errors []string
  ttype string
  key string
  value string
}

func NewProxyIf(input string) *ProxyIf {
  p := &ProxyIf{
    input: strings.TrimSpace(input),
    errors: make([]string, 0),
    ttype: "",
    key: "",
    value: "",
  }
  p.parseInput()
  return p
}

func (pi *ProxyIf) Errors() []string {
  return pi.errors
}

func (pi *ProxyIf) IsEmpty() bool {
  return pi.IsValid() && pi.input == ""
}

func (pi *ProxyIf) IsValid() bool {
  return len(pi.errors) == 0
}

func (pi *ProxyIf) IsBaggage() bool {
  return pi.ttype == ProxyIfType_Baggage
}

func (pi *ProxyIf) IsTag() bool {
  return pi.ttype == ProxyIfType_Tag
}

func (pi *ProxyIf) Key() string {
  return pi.key
}

func (pi *ProxyIf) Value() string {
  return pi.value
}

func (pi *ProxyIf) parseInput() {
  re := regexp.MustCompile("(baggage|tag)\\.(.[^=]*)==(.*)")
  if pi.input != "" {
    matches := re.FindAllStringSubmatch(pi.input, -1)
    if len(matches) != 1 {
      pi.errors = append(pi.errors, fmt.Sprintf("Input '%+v' not valid proxy-if expression.", pi.input))
    } else {
      if ttype, key, value, err := pi.unpack(matches[0]); err == nil {
        pi.ttype = ttype
        pi.key = key
        pi.value = value
      } else {
        pi.errors = append(pi.errors, fmt.Sprintf("%+v", err))
      }
    }
  }
}

func (pi *ProxyIf) unpack(input []string) (string, string, string, error) {
  if len(input) != 4 {
    return "", "", "", errors.New("Input length must be 4 items.")
  } else {
    complete := input[0]
    ttype := input[1]
    key := input[2]
    value := input[3]
    if complete != fmt.Sprintf("%s.%s==%s", ttype, key, value) {
      return "", "", "", errors.New("Input not in the correct format.")
    } else {
      if ttype != ProxyIfType_Baggage && ttype != ProxyIfType_Tag {
        return "", "", "", errors.New("Input type must be either baggage or tag.")
      } else {
        return ttype, key, value, nil
      }
    }
  }
}