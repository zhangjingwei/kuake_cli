package sdk

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
)

func TestEncodeHashCtx(t *testing.T) {
	tests := []struct {
		name string
		ctx  *HashCtx
	}{
		{
			name: "encode valid hash ctx",
			ctx: &HashCtx{
				HashType: "sha1",
				H0:       "1125272656",
				H1:       "2794323374",
				H2:       "1697191688",
				H3:       "2476193098",
				H4:       "2437866605",
				Nl:       "436207616",
				Nh:       "0",
				Data:     "",
				Num:      "0",
			},
		},
		{
			name: "encode hash ctx with different values",
			ctx: &HashCtx{
				HashType: "sha1",
				H0:       "2610988334",
				H1:       "3880416807",
				H2:       "226293050",
				H3:       "2458362688",
				H4:       "1367420798",
				Nl:       "335544320",
				Nh:       "0",
				Data:     "",
				Num:      "0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := encodeHashCtx(tt.ctx)
			if err != nil {
				t.Fatalf("encodeHashCtx() error = %v", err)
			}

			if encoded == "" {
				t.Fatal("encodeHashCtx() returned empty string")
			}

			// 验证编码后的字符串是有效的 base64
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatalf("encoded string is not valid base64: %v", err)
			}

			// 验证解码后的 JSON 可以解析
			var decodedCtx HashCtx
			if err := json.Unmarshal(decoded, &decodedCtx); err != nil {
				t.Fatalf("decoded JSON is invalid: %v", err)
			}

			// 验证字段是否匹配
			if decodedCtx.HashType != tt.ctx.HashType {
				t.Errorf("HashType = %v, want %v", decodedCtx.HashType, tt.ctx.HashType)
			}
			if decodedCtx.H0 != tt.ctx.H0 {
				t.Errorf("H0 = %v, want %v", decodedCtx.H0, tt.ctx.H0)
			}
			if decodedCtx.H1 != tt.ctx.H1 {
				t.Errorf("H1 = %v, want %v", decodedCtx.H1, tt.ctx.H1)
			}
			if decodedCtx.H2 != tt.ctx.H2 {
				t.Errorf("H2 = %v, want %v", decodedCtx.H2, tt.ctx.H2)
			}
			if decodedCtx.H3 != tt.ctx.H3 {
				t.Errorf("H3 = %v, want %v", decodedCtx.H3, tt.ctx.H3)
			}
			if decodedCtx.H4 != tt.ctx.H4 {
				t.Errorf("H4 = %v, want %v", decodedCtx.H4, tt.ctx.H4)
			}
			if decodedCtx.Nl != tt.ctx.Nl {
				t.Errorf("Nl = %v, want %v", decodedCtx.Nl, tt.ctx.Nl)
			}
			if decodedCtx.Nh != tt.ctx.Nh {
				t.Errorf("Nh = %v, want %v", decodedCtx.Nh, tt.ctx.Nh)
			}
			if decodedCtx.Data != tt.ctx.Data {
				t.Errorf("Data = %v, want %v", decodedCtx.Data, tt.ctx.Data)
			}
			if decodedCtx.Num != tt.ctx.Num {
				t.Errorf("Num = %v, want %v", decodedCtx.Num, tt.ctx.Num)
			}
		})
	}
}

func TestEncodeHashCtx_Nil(t *testing.T) {
	encoded, err := encodeHashCtx(nil)
	if err != nil {
		t.Fatalf("encodeHashCtx(nil) error = %v", err)
	}
	if encoded != "" {
		t.Errorf("encodeHashCtx(nil) = %v, want empty string", encoded)
	}
}

func TestUpdateHashCtxFromHash(t *testing.T) {
	tests := []struct {
		name      string
		chunkData []byte
		totalBytes int64
		wantNl    int64
	}{
		{
			name:       "first chunk",
			chunkData:  []byte("test chunk data"),
			totalBytes: 0,
			wantNl:     15, // len("test chunk data")
		},
		{
			name:       "second chunk",
			chunkData:  []byte("more data"),
			totalBytes: 15,
			wantNl:     24, // 15 + 9 (len("more data") = 9, not 10)
		},
		{
			name:       "large chunk",
			chunkData:  make([]byte, 4194304), // 4MB
			totalBytes: 0,
			wantNl:     4194304,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := sha1.New()
			
			// 如果 totalBytes > 0，说明之前已经有数据，需要先处理前面的数据
			// 注意：updateHashCtxFromHash 会再次写入 chunkData，所以这里只写入前面的数据
			if tt.totalBytes > 0 {
				// 模拟前面的数据（这里用空数据，实际应该用真实的前面分片数据）
				// 为了测试，我们直接写入一些数据来模拟
				prevData := make([]byte, tt.totalBytes)
				hash.Write(prevData)
			}

			ctx, err := updateHashCtxFromHash(hash, tt.chunkData, tt.totalBytes)
			if err != nil {
				t.Fatalf("updateHashCtxFromHash() error = %v", err)
			}

			if ctx == nil {
				t.Fatal("updateHashCtxFromHash() returned nil")
			}

			// 验证基本字段
			if ctx.HashType != "sha1" {
				t.Errorf("HashType = %v, want sha1", ctx.HashType)
			}

			// 验证 Nl 字段
			var gotNl int64
			fmt.Sscanf(ctx.Nl, "%d", &gotNl)
			if gotNl != tt.wantNl {
				t.Errorf("Nl = %v (%d), want %d", ctx.Nl, gotNl, tt.wantNl)
			}

			// 验证 Nh, Data, Num 字段
			if ctx.Nh != "0" {
				t.Errorf("Nh = %v, want 0", ctx.Nh)
			}
			if ctx.Data != "" {
				t.Errorf("Data = %v, want empty string", ctx.Data)
			}
			if ctx.Num != "0" {
				t.Errorf("Num = %v, want 0", ctx.Num)
			}

			// 验证 h0-h4 字段不为空且是数字
			if ctx.H0 == "" {
				t.Error("H0 is empty")
			}
			if ctx.H1 == "" {
				t.Error("H1 is empty")
			}
			if ctx.H2 == "" {
				t.Error("H2 is empty")
			}
			if ctx.H3 == "" {
				t.Error("H3 is empty")
			}
			if ctx.H4 == "" {
				t.Error("H4 is empty")
			}

			// 验证 h0-h4 是有效的数字字符串
			var h0, h1, h2, h3, h4 uint32
			if _, err := fmt.Sscanf(ctx.H0, "%d", &h0); err != nil {
				t.Errorf("H0 is not a valid number: %v", err)
			}
			if _, err := fmt.Sscanf(ctx.H1, "%d", &h1); err != nil {
				t.Errorf("H1 is not a valid number: %v", err)
			}
			if _, err := fmt.Sscanf(ctx.H2, "%d", &h2); err != nil {
				t.Errorf("H2 is not a valid number: %v", err)
			}
			if _, err := fmt.Sscanf(ctx.H3, "%d", &h3); err != nil {
				t.Errorf("H3 is not a valid number: %v", err)
			}
			if _, err := fmt.Sscanf(ctx.H4, "%d", &h4); err != nil {
				t.Errorf("H4 is not a valid number: %v", err)
			}
		})
	}
}

func TestUpdateHashCtxFromHash_Incremental(t *testing.T) {
	// 测试增量哈希计算：多个分片连续处理
	hash := sha1.New()
	
	chunks := [][]byte{
		[]byte("chunk1"),
		[]byte("chunk2"),
		[]byte("chunk3"),
	}

	var totalBytes int64
	var contexts []*HashCtx

	for i, chunk := range chunks {
		ctx, err := updateHashCtxFromHash(hash, chunk, totalBytes)
		if err != nil {
			t.Fatalf("updateHashCtxFromHash() error at chunk %d: %v", i+1, err)
		}
		contexts = append(contexts, ctx)
		totalBytes += int64(len(chunk))
	}

	// 验证每个上下文的 Nl 值是否正确递增
	expectedNl := []int64{6, 12, 18} // len("chunk1")=6, +6=12, +6=18
	for i, ctx := range contexts {
		var gotNl int64
		fmt.Sscanf(ctx.Nl, "%d", &gotNl)
		if gotNl != expectedNl[i] {
			t.Errorf("Context %d: Nl = %d, want %d", i+1, gotNl, expectedNl[i])
		}
	}

	// 验证后续分片的哈希值应该不同（因为累积了更多数据）
	if contexts[0].H0 == contexts[1].H0 && contexts[1].H0 == contexts[2].H0 {
		t.Error("Hash values should be different for different chunks")
	}
}

func TestUpdateHashCtxFromHash_Consistency(t *testing.T) {
	// 测试一致性：相同的数据应该产生相同的哈希上下文
	data1 := []byte("test data")
	data2 := []byte("test data")

	hash1 := sha1.New()
	hash2 := sha1.New()

	ctx1, err1 := updateHashCtxFromHash(hash1, data1, 0)
	ctx2, err2 := updateHashCtxFromHash(hash2, data2, 0)

	if err1 != nil {
		t.Fatalf("updateHashCtxFromHash() error = %v", err1)
	}
	if err2 != nil {
		t.Fatalf("updateHashCtxFromHash() error = %v", err2)
	}

	// 相同的数据应该产生相同的哈希值
	if ctx1.H0 != ctx2.H0 {
		t.Errorf("H0 mismatch: %v != %v", ctx1.H0, ctx2.H0)
	}
	if ctx1.H1 != ctx2.H1 {
		t.Errorf("H1 mismatch: %v != %v", ctx1.H1, ctx2.H1)
	}
	if ctx1.H2 != ctx2.H2 {
		t.Errorf("H2 mismatch: %v != %v", ctx1.H2, ctx2.H2)
	}
	if ctx1.H3 != ctx2.H3 {
		t.Errorf("H3 mismatch: %v != %v", ctx1.H3, ctx2.H3)
	}
	if ctx1.H4 != ctx2.H4 {
		t.Errorf("H4 mismatch: %v != %v", ctx1.H4, ctx2.H4)
	}
	if ctx1.Nl != ctx2.Nl {
		t.Errorf("Nl mismatch: %v != %v", ctx1.Nl, ctx2.Nl)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// 测试编码-解码的往返转换
	original := &HashCtx{
		HashType: "sha1",
		H0:       "1125272656",
		H1:       "2794323374",
		H2:       "1697191688",
		H3:       "2476193098",
		H4:       "2437866605",
		Nl:       "436207616",
		Nh:       "0",
		Data:     "",
		Num:      "0",
	}

	// 编码
	encoded, err := encodeHashCtx(original)
	if err != nil {
		t.Fatalf("encodeHashCtx() error = %v", err)
	}

	// 解码
	decodedBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode error = %v", err)
	}

	var decoded HashCtx
	if err := json.Unmarshal(decodedBytes, &decoded); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}

	// 验证所有字段都匹配
	if decoded.HashType != original.HashType {
		t.Errorf("HashType: got %v, want %v", decoded.HashType, original.HashType)
	}
	if decoded.H0 != original.H0 {
		t.Errorf("H0: got %v, want %v", decoded.H0, original.H0)
	}
	if decoded.H1 != original.H1 {
		t.Errorf("H1: got %v, want %v", decoded.H1, original.H1)
	}
	if decoded.H2 != original.H2 {
		t.Errorf("H2: got %v, want %v", decoded.H2, original.H2)
	}
	if decoded.H3 != original.H3 {
		t.Errorf("H3: got %v, want %v", decoded.H3, original.H3)
	}
	if decoded.H4 != original.H4 {
		t.Errorf("H4: got %v, want %v", decoded.H4, original.H4)
	}
	if decoded.Nl != original.Nl {
		t.Errorf("Nl: got %v, want %v", decoded.Nl, original.Nl)
	}
	if decoded.Nh != original.Nh {
		t.Errorf("Nh: got %v, want %v", decoded.Nh, original.Nh)
	}
	if decoded.Data != original.Data {
		t.Errorf("Data: got %v, want %v", decoded.Data, original.Data)
	}
	if decoded.Num != original.Num {
		t.Errorf("Num: got %v, want %v", decoded.Num, original.Num)
	}
}

func TestHashCtx_RealWorldExample(t *testing.T) {
	// 使用真实世界的示例数据测试
	// 这是从浏览器请求中提取的实际 HashCtx 值
	realWorldBase64 := "eyJoYXNoX3R5cGUiOiJzaGExIiwiaDAiOiIxMTI1MjcyNjU2IiwiaDEiOiIyNzk0MzIzMzc0IiwiaDIiOiIxNjk3MTkxNjg4IiwiaDMiOiIyNDc2MTkzMDk4IiwiaDQiOiIyNDM3ODY2NjA1IiwiTmwiOiI0MzYyMDc2MTYiLCJOaCI6IjAiLCJkYXRhIjoiIiwibnVtIjoiMCJ9"
	
	// 解码
	decodedBytes, err := base64.StdEncoding.DecodeString(realWorldBase64)
	if err != nil {
		t.Fatalf("base64 decode error = %v", err)
	}

	var ctx HashCtx
	if err := json.Unmarshal(decodedBytes, &ctx); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}

	// 验证字段
	if ctx.HashType != "sha1" {
		t.Errorf("HashType = %v, want sha1", ctx.HashType)
	}
	if ctx.H0 != "1125272656" {
		t.Errorf("H0 = %v, want 1125272656", ctx.H0)
	}
	if ctx.Nl != "436207616" {
		t.Errorf("Nl = %v, want 436207616", ctx.Nl)
	}

	// 重新编码，应该得到相同的结果
	reEncoded, err := encodeHashCtx(&ctx)
	if err != nil {
		t.Fatalf("encodeHashCtx() error = %v", err)
	}

	if reEncoded != realWorldBase64 {
		t.Errorf("Re-encoded value differs from original")
		t.Logf("Original: %s", realWorldBase64)
		t.Logf("Re-encoded: %s", reEncoded)
	}
}
