package services

import (
	"bytes"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	goldhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var mentionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var commentMentionKind = ast.NewNodeKind("CommentMention")

type commentMentionNode struct {
	ast.BaseInline
	id, label string
}

func (n *commentMentionNode) Kind() ast.NodeKind { return commentMentionKind }
func (n *commentMentionNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"UserID": n.id}, nil)
}

type commentMentionRenderer struct{}

func (r *commentMentionRenderer) RegisterFuncs(register renderer.NodeRendererFuncRegisterer) {
	register.Register(commentMentionKind, func(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		mention := node.(*commentMentionNode)
		// Labels and IDs are source text, never trusted HTML. Mentions are not
		// profile links, permission grants, or a way to fetch another identity.
		_, err := w.WriteString(`<span class="comment-mention" data-user-id="` + html.EscapeString(mention.id) + `">` + html.EscapeString(mention.label) + `</span>`)
		return ast.WalkContinue, err
	})
}

// RenderCommentMarkdown shares SaaS's Goldmark safe defaults: no raw HTML or
// unsafe links. Images become their alt text to avoid remote tracking requests.
// Source remains untouched; the HTML is a read/preview projection only.
func RenderCommentMarkdown(body string) (string, []string, error) {
	render, ids := prepareCommentMarkdown(body)
	rendered, err := render(nil)
	return rendered, ids, err
}

func prepareCommentMarkdown(body string) (func(map[string]string) (string, error), []string) {
	source := []byte(body)
	markdown := goldmark.New(
		goldmark.WithRendererOptions(goldhtml.WithHardWraps(),
			renderer.WithNodeRenderers(util.Prioritized(&commentMentionRenderer{}, 500))),
	)
	document := markdown.Parser().Parse(text.NewReader(source))
	var replace []ast.Node
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Image:
			replace = append(replace, n)
			return ast.WalkSkipChildren, nil
		case *ast.Link:
			if strings.HasPrefix(string(n.Destination), "mention:") {
				replace = append(replace, n)
				return ast.WalkSkipChildren, nil
			}
		}
		return ast.WalkContinue, nil
	})
	ids, seen := []string{}, map[string]bool{}
	var mentions []*commentMentionNode
	for _, node := range replace {
		label := html.UnescapeString(string(util.UnescapePunctuations(node.Text(source))))
		var replacement ast.Node = ast.NewString([]byte(label))
		if link, ok := node.(*ast.Link); ok {
			id := strings.TrimPrefix(string(link.Destination), "mention:")
			if mentionIDPattern.MatchString(id) && strings.HasPrefix(label, "@") && len(label) <= 200 {
				mention := &commentMentionNode{id: id, label: label}
				mentions = append(mentions, mention)
				replacement = mention
				if !seen[id] {
					ids = append(ids, id)
					seen[id] = true
				}
			}
		}
		node.Parent().ReplaceChild(node.Parent(), node, replacement)
	}
	return func(names map[string]string) (string, error) {
		for _, mention := range mentions {
			if name := names[mention.id]; name != "" {
				mention.label = "@" + name
			}
		}
		var rendered bytes.Buffer
		err := markdown.Renderer().Render(&rendered, source, document)
		return rendered.String(), err
	}, ids
}
