package permission

// DangerousInterpreters 高风险代码执行命令。
var DangerousInterpreters = []string{
	"python", "python3", "node", "deno", "ruby", "perl", "php",
	"bash -c", "sh -c", "zsh -c", "fish -c",
	"eval", "exec", "xargs",
	"npx", "bunx",
}

// DangerousNetworkCommands 网络外传/远程执行命令。
var DangerousNetworkCommands = []string{
	"curl", "wget", "ssh", "scp", "rsync", "nc", "ncat", "socat",
}

// DangerousCloudCommands 云平台 CLI。
var DangerousCloudCommands = []string{
	"aws", "gcloud", "gsutil", "kubectl", "helm", "terraform", "pulumi", "az",
}

// DangerousPrivilegedCommands 提权命令。
var DangerousPrivilegedCommands = []string{
	"sudo", "su", "chown", "doas",
}

// DefaultAskRules 返回基于危险模式的 ask 规则。
// 标记为 ask（非 deny）：用户可确认或通过 config allow 豁免。
func DefaultAskRules() []Rule {
	var rules []Rule
	allPatterns := [][]string{
		DangerousInterpreters,
		DangerousNetworkCommands,
		DangerousCloudCommands,
		DangerousPrivilegedCommands,
	}
	for _, patterns := range allPatterns {
		for _, p := range patterns {
			rules = append(rules, Rule{Tool: "Bash", Pattern: p + " *", Decision: Ask})
		}
	}
	return rules
}
