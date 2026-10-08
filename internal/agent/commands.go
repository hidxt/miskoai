package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hidxt/miskoai/internal/storage"
)

func searchCommand(text string) (string, bool) {
	if strings.HasPrefix(text, "/search ") {
		return strings.TrimSpace(strings.TrimPrefix(text, "/search ")), true
	}
	if text == "/search" {
		return "", true
	}
	if strings.HasPrefix(text, "搜索：") {
		return strings.TrimSpace(strings.TrimPrefix(text, "搜索：")), true
	}
	return "", false
}

// encodeFacts retains only a channel-bounded output buffer and one encoded
// fact. Store field caps bound that temporary fact even when JSON escapes
// expand its text. Stop immediately rather than encode the remaining slice.
func encodeFacts(facts []storage.Fact) ([]byte, bool, error) {
	if len(facts) == 0 {
		return []byte("[]"), true, nil
	}
	out := make([]byte, 1, maxReply)
	out[0] = '['
	for i, fact := range facts {
		encoded, err := json.Marshal(fact)
		if err != nil {
			return nil, false, err
		}
		separator := 0
		if i > 0 {
			separator = 1
		}
		if len(out)+separator+len(encoded)+1 > maxReply {
			return nil, false, nil
		}
		if separator != 0 {
			out = append(out, ',')
		}
		out = append(out, encoded...)
	}
	out = append(out, ']')
	return out, true, nil
}

func (a *Agent) command(ctx context.Context, text string) (string, string, bool) {
	switch {
	case strings.HasPrefix(text, "/remember ") || strings.HasPrefix(text, "请记住：") || text == "/remember":
		content := strings.TrimPrefix(text, "/remember ")
		if strings.HasPrefix(text, "请记住：") {
			content = strings.TrimPrefix(text, "请记住：")
		}
		if text == "/remember" || strings.TrimSpace(content) == "" {
			return "请提供要记住的内容。", "input", true
		}
		id, err := a.store.AddFact(ctx, a.scope, storage.Fact{Content: content, Source: "explicit_user", Confidence: 1, Importance: 50})
		if err != nil {
			return memoryReply, storageCode(err), true
		}
		return fmt.Sprintf("已记住，记忆编号：%d。", id), "", true
	case text == "/memory clear":
		if err := a.store.ClearFacts(ctx, a.scope); err != nil {
			return memoryReply, storageCode(err), true
		}
		return "已清空你的明确记忆。", "", true
	case text == "/export-memory":
		facts, err := a.store.ExportFacts(ctx, a.scope)
		if err != nil {
			return memoryReply, storageCode(err), true
		}
		b, fits, err := encodeFacts(facts)
		if err != nil {
			return memoryReply, "storage", true
		}
		if !fits {
			return "记忆导出超过消息长度限制。完整导出需要后续的管理功能；该功能目前尚未提供。", "limit", true
		}
		return string(b), "", true
	case text == "/memory" || strings.HasPrefix(text, "/memory "):
		query := ""
		if text != "/memory" {
			query = strings.TrimPrefix(text, "/memory ")
		}
		facts, err := a.store.SearchFacts(ctx, a.scope, factQuery(query), 8)
		if err != nil {
			return memoryReply, storageCode(err), true
		}
		if len(facts) == 0 {
			return "没有找到匹配的明确记忆。", "", true
		}
		var b strings.Builder
		for _, f := range facts {
			fmt.Fprintf(&b, "%d：%s\n", f.ID, prefixBytes(f.Content, 1600))
		}
		return b.String(), "", true
	case text == "/forget" || strings.HasPrefix(text, "/forget "):
		id, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(text, "/forget ")), 10, 64)
		if err != nil || id <= 0 {
			return "请提供有效的记忆编号。", "input", true
		}
		if err = a.store.DeleteFact(ctx, a.scope, id); err != nil {
			if err == storage.ErrNotFound {
				return "没有找到该记忆编号。", "", true
			}
			return memoryReply, storageCode(err), true
		}
		return "已删除该条明确记忆。", "", true
	}
	return "", "", false
}
