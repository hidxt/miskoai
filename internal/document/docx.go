package document

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

const (
	contentTypesNS  = "http://schemas.openxmlformats.org/package/2006/content-types"
	relationshipsNS = "http://schemas.openxmlformats.org/package/2006/relationships"
	wordNS          = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	strictWordNS    = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	officeRel       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	strictOfficeRel = "http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument"
	mainContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	compatibilityNS = "http://schemas.openxmlformats.org/markup-compatibility/2006"
)

func extractDOCX(ctx context.Context, input io.ReaderAt, size int64) (Result, error) {
	manifest, e := admitZIP(ctx, input, size)
	if e != nil {
		return Result{}, e
	}
	parts, e := validateZIPMembers(ctx, input, manifest, map[string]bool{"[Content_Types].xml": true, "_rels/.rels": true, "word/document.xml": true})
	if e != nil {
		return Result{}, e
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
		if len(parts[name]) == 0 {
			return Result{}, ErrInvalid
		}
	}
	if e = validateContentTypes(ctx, parts["[Content_Types].xml"]); e != nil {
		return Result{}, e
	}
	rel, e := validateRelationships(ctx, parts["_rels/.rels"])
	if e != nil {
		return Result{}, e
	}
	text, pages, e := extractMainStory(ctx, parts["word/document.xml"], rel)
	if e != nil {
		return Result{}, e
	}
	if e = checkWork(ctx); e != nil {
		return Result{}, e
	}
	return Result{Text: text, Pages: pages, Format: DOCX}, nil
}
func xmlDecoder(ctx context.Context, b []byte) *xml.Decoder {
	d := xml.NewDecoder(newXMLGuard(ctx, bytes.NewReader(b)))
	d.Strict = true
	d.Entity = nil
	d.CharsetReader = nil
	return d
}
func safeXMLToken(ctx context.Context, d *xml.Decoder) (xml.Token, error) {
	if e := checkWork(ctx); e != nil {
		return nil, e
	}
	t, e := d.Token()
	if workErr := checkWork(ctx); workErr != nil {
		return nil, workErr
	}
	if e != nil {
		if e == io.EOF {
			return nil, e
		}
		for _, safe := range []error{ErrLimit, ErrUnsupported, ErrCanceled} {
			if errors.Is(e, safe) {
				return nil, e
			}
		}
		return nil, ErrInvalid
	}
	if start, ok := t.(xml.StartElement); ok {
		seen := make(map[xml.Name]bool, len(start.Attr))
		for _, a := range start.Attr {
			if seen[a.Name] {
				return nil, ErrInvalid
			}
			seen[a.Name] = true
		}
	}
	if _, ok := t.(xml.Directive); ok {
		return nil, ErrUnsupported
	}
	return t, nil
}
func plainAttr(s xml.StartElement, name string) (string, bool) {
	for _, a := range s.Attr {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

// Package metadata admits only the fixed main override. Other normal package
// declarations are validated structurally but never become input paths.
func validateContentTypes(ctx context.Context, b []byte) error {
	d := xmlDecoder(ctx, b)
	depth := 0
	root, main := false, false
	for {
		tok, e := safeXMLToken(ctx, d)
		if e == io.EOF {
			if !root || depth != 0 || !main {
				return ErrInvalid
			}
			return nil
		}
		if e != nil {
			return e
		}
		switch v := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				if root || v.Name != (xml.Name{Space: contentTypesNS, Local: "Types"}) {
					return ErrInvalid
				}
				root = true
			} else if depth == 2 {
				if v.Name.Space != contentTypesNS {
					return ErrInvalid
				}
				switch v.Name.Local {
				case "Override":
					part, ok := plainAttr(v, "PartName")
					ct, has := plainAttr(v, "ContentType")
					if !ok || !has {
						return ErrInvalid
					}
					if part == "/word/document.xml" {
						if main {
							return ErrInvalid
						}
						if ct != mainContentType {
							return ErrUnsupported
						}
						main = true
					} else if mainStoryContentType(ct) {
						return ErrUnsupported
					}
				case "Default":
					if _, ok := plainAttr(v, "Extension"); !ok {
						return ErrInvalid
					}
					if ct, ok := plainAttr(v, "ContentType"); !ok {
						return ErrInvalid
					} else if mainStoryContentType(ct) {
						return ErrUnsupported
					}
				default:
					return ErrInvalid
				}
			} else {
				return ErrInvalid
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if !xmlWhitespace(v) {
				return ErrInvalid
			}
		}
	}
}
func validateRelationships(ctx context.Context, b []byte) (string, error) {
	d := xmlDecoder(ctx, b)
	depth := 0
	root := false
	main := ""
	ids := map[string]bool{}
	for {
		tok, e := safeXMLToken(ctx, d)
		if e == io.EOF {
			if !root || depth != 0 || main == "" {
				return "", ErrInvalid
			}
			return main, nil
		}
		if e != nil {
			return "", e
		}
		switch v := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				if root || v.Name != (xml.Name{Space: relationshipsNS, Local: "Relationships"}) {
					return "", ErrInvalid
				}
				root = true
			} else if depth == 2 {
				if v.Name != (xml.Name{Space: relationshipsNS, Local: "Relationship"}) {
					return "", ErrInvalid
				}
				id, ok := plainAttr(v, "Id")
				typ, has := plainAttr(v, "Type")
				target, present := plainAttr(v, "Target")
				if !ok || !has || !present || !canonicalRelationshipID(id) || ids[id] {
					return "", ErrInvalid
				}
				ids[id] = true
				mode, hasMode := plainAttr(v, "TargetMode")
				if hasMode && mode != "Internal" {
					return "", ErrUnsupported
				}
				if typ == officeRel || typ == strictOfficeRel {
					if main != "" || target != "word/document.xml" && target != "/word/document.xml" {
						return "", ErrInvalid
					}
					main = typ
				}
			} else {
				return "", ErrInvalid
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if !xmlWhitespace(v) {
				return "", ErrInvalid
			}
		}
	}
}
func canonicalRelationshipID(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' || i > 0 && (r >= '0' && r <= '9' || r == '-' || r == '.')) {
			return false
		}
	}
	return true
}

// Only the main document/body story is supported. Graphics, headers, notes,
// field instructions and imported/alternate stories are never resolved.
func extractMainStory(ctx context.Context, b []byte, rel string) (string, int, error) {
	d := xmlDecoder(ctx, b)
	stack := make([]xml.Name, 0, maxXMLDepth)
	root, body := false, false
	bodyDepth, skipDepth, textDepth := 0, 0, 0
	ns := ""
	pages := 1
	var output []byte
	appendText := func(s []byte) error { var e error; output, e = appendOutput(ctx, output, s); return e }
	for {
		tok, e := safeXMLToken(ctx, d)
		if e == io.EOF {
			if !root || !body || len(stack) != 0 {
				return "", 0, ErrInvalid
			}
			if strings.TrimSpace(string(output)) == "" {
				return "", 0, ErrEmpty
			}
			return string(output), pages, nil
		}
		if e != nil {
			return "", 0, e
		}
		switch v := tok.(type) {
		case xml.StartElement:
			depth := len(stack) + 1
			var parent xml.Name
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			stack = append(stack, v.Name)
			if depth == 1 {
				if root || v.Name.Local != "document" || v.Name.Space != wordNS && v.Name.Space != strictWordNS {
					return "", 0, ErrInvalid
				}
				ns = v.Name.Space
				if ns == wordNS && rel != officeRel || ns == strictWordNS && rel != strictOfficeRel {
					return "", 0, ErrInvalid
				}
				root = true
				continue
			}
			if v.Name.Space == compatibilityNS && v.Name.Local == "AlternateContent" {
				return "", 0, ErrUnsupported
			}
			if (v.Name.Space == wordNS || v.Name.Space == strictWordNS) && v.Name.Space != ns {
				return "", 0, ErrInvalid
			}
			if skipDepth != 0 {
				continue
			}
			if textDepth != 0 || parent.Space == ns && (parent.Local == "tab" || parent.Local == "br") {
				return "", 0, ErrInvalid
			}
			if v.Name.Space != ns {
				if v.Name.Local == "t" || v.Name.Local == "body" || v.Name.Local == "document" {
					return "", 0, ErrInvalid
				}
				skipDepth = depth
				continue
			}
			if v.Name.Local == "body" {
				if depth != 2 || body {
					return "", 0, ErrInvalid
				}
				body = true
				bodyDepth = depth
				continue
			}
			if v.Name.Local == "document" {
				return "", 0, ErrInvalid
			}
			if bodyDepth == 0 {
				if depth == 2 && v.Name.Local == "background" {
					skipDepth = depth
					continue
				}
				return "", 0, ErrUnsupported
			}
			switch v.Name.Local {
			case "altChunk", "subDoc", "fldSimple", "instrText", "fldChar":
				return "", 0, ErrUnsupported
			case "p":
				if !wordParent(parent, ns, "body", "tc", "sdtContent", "ins") || hasWordAncestor(stack[:len(stack)-1], ns, "p") || hasWordAncestor(stack[:len(stack)-1], ns, "r") {
					return "", 0, ErrInvalid
				}
			case "r":
				if !wordParent(parent, ns, "p", "hyperlink", "sdtContent", "ins", "smartTag") || !hasWordAncestor(stack, ns, "p") || hasWordAncestor(stack[:len(stack)-1], ns, "r") {
					return "", 0, ErrInvalid
				}
			case "tbl":
				if !wordParent(parent, ns, "body", "tc", "sdtContent", "ins") {
					return "", 0, ErrInvalid
				}
			case "tr":
				if !wordParent(parent, ns, "tbl", "sdtContent", "ins") {
					return "", 0, ErrInvalid
				}
			case "tc":
				if !wordParent(parent, ns, "tr", "sdtContent", "ins") {
					return "", 0, ErrInvalid
				}
			case "hyperlink":
				if !wordParent(parent, ns, "p", "sdtContent", "ins") {
					return "", 0, ErrInvalid
				}
			case "sdt":
				if !wordParent(parent, ns, "body", "tc", "p", "tbl", "tr", "sdtContent") {
					return "", 0, ErrInvalid
				}
			case "sdtContent":
				if !wordParent(parent, ns, "sdt") {
					return "", 0, ErrInvalid
				}
			case "ins":
				if !wordParent(parent, ns, "body", "tc", "p", "tbl", "tr", "hyperlink", "sdtContent") {
					return "", 0, ErrInvalid
				}
			case "smartTag":
				if !wordParent(parent, ns, "p", "smartTag", "hyperlink", "sdtContent") {
					return "", 0, ErrInvalid
				}
			case "t":
				if parent != (xml.Name{Space: ns, Local: "r"}) || textDepth != 0 || !hasWordAncestor(stack, ns, "p") {
					return "", 0, ErrInvalid
				}
				textDepth = depth
			case "tab", "br":
				if parent != (xml.Name{Space: ns, Local: "r"}) || !hasWordAncestor(stack, ns, "p") {
					return "", 0, ErrInvalid
				}
				if v.Name.Local == "tab" {
					e = appendText([]byte{'\t'})
				} else {
					typ := ""
					for _, a := range v.Attr {
						if a.Name == (xml.Name{Space: ns, Local: "type"}) {
							typ = a.Value
						}
					}
					if typ != "" && typ != "textWrapping" && typ != "page" {
						return "", 0, ErrUnsupported
					}
					if typ == "page" {
						pages++
						if pages > 64 {
							return "", 0, ErrLimit
						}
					}
					e = appendText([]byte{'\n'})
				}
				if e != nil {
					return "", 0, e
				}
			default:
				excluded, validParent := excludedWordSubtree(v.Name.Local, parent, ns)
				if !excluded {
					// Unknown WordML may contain main-story text (customXml,
					// moveTo and future wrappers). Never return a prefix while
					// silently skipping that unimplemented story content.
					return "", 0, ErrUnsupported
				}
				if !validParent {
					return "", 0, ErrInvalid
				}
				skipDepth = depth
			}
		case xml.EndElement:
			depth := len(stack)
			if skipDepth == depth {
				skipDepth = 0
			} else if skipDepth == 0 {
				if textDepth == depth {
					textDepth = 0
				}
				if bodyDepth != 0 && v.Name == (xml.Name{Space: ns, Local: "p"}) {
					if e = appendText([]byte{'\n'}); e != nil {
						return "", 0, e
					}
				}
				if bodyDepth == depth {
					bodyDepth = 0
				}
			}
			stack = stack[:depth-1]
		case xml.CharData:
			if skipDepth == 0 && textDepth != 0 {
				if len(stack) != textDepth {
					return "", 0, ErrInvalid
				}
				if e = appendText(v); e != nil {
					return "", 0, e
				}
			} else if skipDepth == 0 && !xmlWhitespace(v) {
				return "", 0, ErrInvalid
			}
		}
	}
}
func mainStoryContentType(s string) bool {
	return s == mainContentType || (strings.Contains(s, "wordprocessingml") || strings.Contains(s, "ms-word")) && strings.Contains(s, ".main+")
}
func xmlWhitespace(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return false
		}
	}
	return true
}
func hasWordAncestor(stack []xml.Name, ns, local string) bool {
	for _, n := range stack {
		if n == (xml.Name{Space: ns, Local: local}) {
			return true
		}
	}
	return false
}
func wordParent(parent xml.Name, ns string, names ...string) bool {
	if parent.Space != ns {
		return false
	}
	for _, name := range names {
		if parent.Local == name {
			return true
		}
	}
	return false
}

// This is the explicit non-story exclusion whitelist. Descendants remain
// XML-guarded, but are not searched for text. New WordML names must receive an
// explicit supported or excluded policy; defaulting to skip would lose text.
func excludedWordSubtree(local string, parent xml.Name, ns string) (bool, bool) {
	switch local {
	case "pPr":
		return true, wordParent(parent, ns, "p")
	case "rPr":
		return true, wordParent(parent, ns, "r")
	case "tblPr", "tblGrid":
		return true, wordParent(parent, ns, "tbl")
	case "trPr":
		return true, wordParent(parent, ns, "tr")
	case "tcPr":
		return true, wordParent(parent, ns, "tc")
	case "sdtPr", "sdtEndPr":
		return true, wordParent(parent, ns, "sdt")
	case "sectPr":
		return true, wordParent(parent, ns, "body")
	case "drawing", "pict", "object", "delText", "footnoteReference", "endnoteReference", "commentReference", "lastRenderedPageBreak":
		return true, wordParent(parent, ns, "r")
	case "del":
		return true, wordParent(parent, ns, "body", "tc", "p", "tbl", "tr", "hyperlink", "sdtContent")
	default:
		return false, false
	}
}
