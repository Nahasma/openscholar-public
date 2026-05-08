package provider

import "testing"

func TestSanitizeContentDelta(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "MiniMax closing tag stripped",
			input: "some text</minimax:tool_call>more text",
			want:  "some textmore text",
		},
		{
			name:  "DeepSeek open and close tags stripped",
			input: "<deepseek:search>query</deepseek:search>",
			want:  "query",
		},
		{
			name:  "Normal text unaffected",
			input: "This is a normal response with no special tags.",
			want:  "This is a normal response with no special tags.",
		},
		{
			name:  "Orphan tool_call closing tag stripped",
			input: "result</tool_call>",
			want:  "result",
		},
		{
			name:  "Orphan function_call closing tag stripped",
			input: "result</function_call>",
			want:  "result",
		},
		{
			name:  "Orphan tool_use closing tag stripped",
			input: "result</tool_use>",
			want:  "result",
		},
		{
			name:  "User legitimate XML not affected",
			input: "<div>hello</div>",
			want:  "<div>hello</div>",
		},
		{
			name:  "Empty string returns empty string",
			input: "",
			want:  "",
		},
		{
			name:  "Zhipu tag stripped",
			input: "text<zhipu:tool_use>payload</zhipu:tool_use>end",
			want:  "textpayloadend",
		},
		{
			name:  "Moonshot tag stripped",
			input: "<moonshot:function_call>data</moonshot:function_call>",
			want:  "data",
		},
		{
			name:  "Baichuan tag stripped",
			input: "prefix<baichuan:tool_call/>suffix",
			want:  "prefixsuffix",
		},
		{
			name:  "Multiple provider tags stripped",
			input: "<minimax:tool_call>foo</minimax:tool_call><deepseek:search>bar</deepseek:search>",
			want:  "foobar",
		},
		{
			name:  "MiniMax invoke block stripped entirely",
			input: `前面的文字<invoke name="ScholarSearch"><parameter name="action">search</parameter><parameter name="query">multi-agent</parameter></invoke>后面的文字`,
			want:  "前面的文字后面的文字",
		},
		{
			name:  "Multiple invoke blocks stripped",
			input: `text<invoke name="A"><parameter name="x">1</parameter></invoke>mid<invoke name="B"><parameter name="y">2</parameter></invoke>end`,
			want:  "textmidend",
		},
		{
			name:  "Legitimate div tags not affected by invoke regex",
			input: `<div class="test">content</div>`,
			want:  `<div class="test">content</div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeContentDelta(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeContentDelta(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
