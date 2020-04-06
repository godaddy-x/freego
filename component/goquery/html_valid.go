package goquery

import (
	"fmt"
	"github.com/godaddy-x/freego/util"
	"strings"
)

var (
	access_tag   = []string{"h4", "h2", "section"}
	access_style = []string{"text-decoration", "line-through", "font-style", "color", "text-align", "font-weight"}
)

type HtmlValidResult struct {
	NewContent string
	ContentLen int
	FailMsg    string
}

func ValidImgURL(content, prefix string) error {
	if strings.HasPrefix(content, prefix) {
		if util.ValidPattern(strings.ReplaceAll(content, prefix, ""), "\\d{19}/\\d{19}\\.jpg") {
			return nil
		}
	}
	return util.Error("图片URL无效")
}

func ValidZxHtml(htmlstr string) (*HtmlValidResult) {
	r := strings.NewReader(util.AddStr("<content>", htmlstr, "</content>"))
	doc, err := NewDocumentFromReader(r)
	if err != nil {
		fmt.Println(err)
		return &HtmlValidResult{FailMsg: "解析html数据失败"}
	}
	children := doc.Find("content").Children()
	if children.Length() == 0 {
		return &HtmlValidResult{FailMsg: "无匹配数据"}
	}
	validResult := &HtmlValidResult{}
	children.Each(func(i int, v *Selection) {
		if len(validResult.FailMsg) > 0 {
			return
		}
		// 样式校验
		tag := ""
		style := ""
		for _, v := range v.Nodes {
			if !util.CheckStr(v.Data, access_tag...) {
				validResult.FailMsg = "Tag类型无效"
				return
			}
			tag = v.Data
			if len(v.Attr) == 0 {
				continue
			}
			if len(v.Attr) > 1 {
				validResult.FailMsg = "无效的样式"
				return
			}
			attr := v.Attr[0]
			if attr.Key != "style" {
				validResult.FailMsg = "样式校验失败"
				return
			}
			style = attr.Val
			split := strings.Split(attr.Val, ";")
			for _, v := range split {
				if len(v) == 0 {
					continue
				}
				split2 := strings.Split(v, ":")
				if len(split2) == 2 {
					if !util.CheckStr(strings.TrimSpace(split2[0]), access_style...) {
						validResult.FailMsg = "不支持的样式"
						return
					}
				} else {
					validResult.FailMsg = "样式异常"
					return
				}
			}
		}
		// 内容校验
		content := []rune(v.Text())
		content_len := len(content)
		new_content := make([]rune, 0, content_len+16)
		for i := 0; i < content_len; i++ {
			v := content[i]
			if v == '<' {
				new_content = append(new_content, '＜')
			} else if v == '>' {
				new_content = append(new_content, '＞')
			} else if v == '\'' {
				new_content = append(new_content, '‘')
			} else if v == '"' {
				new_content = append(new_content, '“')
			} else if v == '&' {
				new_content = append(new_content, '＆')
			} else if v == '\\' {
				new_content = append(new_content, '＼')
			} else if v == '#' {
				new_content = append(new_content, '＃')
			} else if v == ':' {
				new_content = append(new_content, '：')
			} else if v == ';' {
				new_content = append(new_content, '；')
			} else if v == '.' {
				new_content = append(new_content, '。')
			} else if v == '%' {
				if content_len >= i+2 {
					if content[i+1] == '3' && (content[i+2] == 'c' || content[i+2] == 'C') {
						new_content = append(new_content, '＜')
						i += 2
						continue
					}
					if content[i+1] == '6' && content[i+2] == '0' {
						new_content = append(new_content, '＜')
						i += 2
						continue
					}
					if content[i+1] == '3' && (content[i+2] == 'e' || content[i+2] == 'E') {
						new_content = append(new_content, '＞')
						i += 2
						continue
					}
					if content[i+1] == '6' && content[i+2] == '2' {
						new_content = append(new_content, '＞')
						i += 2
						continue
					}
				}
			} else {
				new_content = append(new_content, v)
			}
		}
		if len(style) > 0 {
			style = util.AddStr(" style='", style, "'")
		}
		validResult.ContentLen = validResult.ContentLen + util.Len(strings.TrimSpace(v.Text()))
		validResult.NewContent = util.AddStr(validResult.NewContent, "<", tag, style, ">", string(new_content), "</", tag, ">")
	})
	if len(validResult.FailMsg) > 0 {
		validResult.ContentLen = 0
		validResult.NewContent = ""
	}
	return validResult
}
