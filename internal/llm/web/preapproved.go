package web

// Package web — preapproved domain whitelist for the WebFetch tool.
//
// SECURITY WARNING: These preapproved domains are ONLY for WebFetch (GET
// requests only). The list must NOT be reused for sandbox network rules or
// any write-capable HTTP operation, because some domains (e.g. huggingface.co,
// kaggle.com) allow file uploads and would enable data exfiltration.
//
// Strictly replicated from CC's preapproved.ts, with additional academic
// domains relevant to OpenScholar's use case.

import (
	"net/url"
	"strings"
)

// preapprovedEntries is the combined whitelist: plain hostname OR
// "hostname/path-prefix" for path-scoped entries (e.g. "github.com/anthropics").
// Aligned 1:1 with CC's PREAPPROVED_HOSTS set plus OpenScholar academic additions.
var preapprovedEntries = []string{
	// ── Anthropic ──────────────────────────────────────────────────────────
	"platform.claude.com",
	"code.claude.com",
	"modelcontextprotocol.io",
	"github.com/anthropics", // path-scoped
	"agentskills.io",

	// ── Top Programming Languages ──────────────────────────────────────────
	"docs.python.org",        // Python
	"en.cppreference.com",    // C/C++ reference
	"docs.oracle.com",        // Java
	"learn.microsoft.com",    // C#/.NET / Azure
	"developer.mozilla.org",  // JavaScript/Web APIs (MDN)
	"go.dev",                 // Go
	"pkg.go.dev",             // Go package docs
	"www.php.net",            // PHP
	"docs.swift.org",         // Swift
	"kotlinlang.org",         // Kotlin
	"ruby-doc.org",           // Ruby
	"doc.rust-lang.org",      // Rust
	"www.typescriptlang.org", // TypeScript

	// ── Web & JavaScript Frameworks / Libraries ───────────────────────────
	"react.dev",        // React
	"angular.io",       // Angular
	"vuejs.org",        // Vue.js
	"nextjs.org",       // Next.js
	"expressjs.com",    // Express.js
	"nodejs.org",       // Node.js
	"bun.sh",           // Bun
	"jquery.com",       // jQuery
	"getbootstrap.com", // Bootstrap
	"tailwindcss.com",  // Tailwind CSS
	"d3js.org",         // D3.js
	"threejs.org",      // Three.js
	"redux.js.org",     // Redux
	"webpack.js.org",   // Webpack
	"jestjs.io",        // Jest
	"reactrouter.com",  // React Router

	// ── Python Frameworks & Libraries ─────────────────────────────────────
	"docs.djangoproject.com",    // Django
	"flask.palletsprojects.com", // Flask
	"fastapi.tiangolo.com",      // FastAPI
	"pandas.pydata.org",         // Pandas
	"numpy.org",                 // NumPy
	"www.tensorflow.org",        // TensorFlow
	"pytorch.org",               // PyTorch
	"scikit-learn.org",          // Scikit-learn
	"matplotlib.org",            // Matplotlib
	"requests.readthedocs.io",   // Requests
	"jupyter.org",               // Jupyter

	// ── PHP Frameworks ─────────────────────────────────────────────────────
	"laravel.com",   // Laravel
	"symfony.com",   // Symfony
	"wordpress.org", // WordPress

	// ── Java Frameworks & Libraries ────────────────────────────────────────
	"docs.spring.io",    // Spring
	"hibernate.org",     // Hibernate
	"tomcat.apache.org", // Tomcat
	"gradle.org",        // Gradle
	"maven.apache.org",  // Maven

	// ── .NET & C# Frameworks ───────────────────────────────────────────────
	"asp.net",              // ASP.NET
	"dotnet.microsoft.com", // .NET
	"nuget.org",            // NuGet
	"blazor.net",           // Blazor

	// ── Mobile Development ─────────────────────────────────────────────────
	"reactnative.dev",       // React Native
	"docs.flutter.dev",      // Flutter
	"developer.apple.com",   // iOS/macOS
	"developer.android.com", // Android

	// ── Data Science & Machine Learning ────────────────────────────────────
	"keras.io",         // Keras
	"spark.apache.org", // Apache Spark
	"huggingface.co",   // Hugging Face
	"www.kaggle.com",   // Kaggle

	// ── Databases ──────────────────────────────────────────────────────────
	"www.mongodb.com",    // MongoDB
	"redis.io",           // Redis
	"www.postgresql.org", // PostgreSQL
	"dev.mysql.com",      // MySQL
	"www.sqlite.org",     // SQLite
	"graphql.org",        // GraphQL
	"prisma.io",          // Prisma

	// ── Cloud & DevOps ─────────────────────────────────────────────────────
	"docs.aws.amazon.com",  // AWS
	"cloud.google.com",     // Google Cloud
	"kubernetes.io",        // Kubernetes
	"www.docker.com",       // Docker
	"www.terraform.io",     // Terraform
	"www.ansible.com",      // Ansible
	"vercel.com/docs",      // Vercel (path-scoped)
	"docs.netlify.com",     // Netlify
	"devcenter.heroku.com", // Heroku

	// ── Testing & Monitoring ───────────────────────────────────────────────
	"cypress.io",   // Cypress
	"selenium.dev", // Selenium

	// ── Game Development ───────────────────────────────────────────────────
	"docs.unity.com",        // Unity
	"docs.unrealengine.com", // Unreal Engine

	// ── Other Essential Tools ──────────────────────────────────────────────
	"git-scm.com",      // Git
	"nginx.org",        // Nginx
	"httpd.apache.org", // Apache HTTP Server

	// ── OpenScholar Academic Supplements ──────────────────────────────────
	// Additional domains used by OpenScholar for academic literature access.
	"arxiv.org",               // arXiv preprints
	"scholar.google.com",      // Google Scholar
	"www.semanticscholar.org", // Semantic Scholar
	"api.semanticscholar.org", // Semantic Scholar API
	"dl.acm.org",              // ACM Digital Library
	"ieeexplore.ieee.org",     // IEEE Xplore
	"openreview.net",          // OpenReview (ICLR, NeurIPS, etc.)
	"proceedings.neurips.cc",  // NeurIPS proceedings
	"proceedings.mlr.press",   // PMLR (ICML, AISTATS, etc.)
	"aclanthology.org",        // ACL Anthology (NLP/CL papers)
	"dblp.org",                // DBLP CS bibliography
	"paperswithcode.com",      // Papers With Code
	"pubmed.ncbi.nlm.nih.gov", // PubMed / NCBI
}

// hostnameOnly is an O(1) map for pure-hostname entries.
// Populated once at init time.
var hostnameOnly map[string]struct{}

// pathPrefixes maps hostname → list of required path prefixes for
// path-scoped entries (e.g. "github.com/anthropics" → ["/anthropics"]).
// Populated once at init time.
var pathPrefixes map[string][]string

func init() {
	hostnameOnly = make(map[string]struct{}, len(preapprovedEntries))
	pathPrefixes = make(map[string][]string)

	for _, entry := range preapprovedEntries {
		slash := strings.IndexByte(entry, '/')
		if slash == -1 {
			// Plain hostname: direct set membership.
			hostnameOnly[entry] = struct{}{}
		} else {
			// Path-scoped: split into host and path prefix.
			host := entry[:slash]
			path := entry[slash:] // includes the leading "/"
			pathPrefixes[host] = append(pathPrefixes[host], path)
		}
	}
}

// IsPreapprovedHost reports whether the given hostname + pathname combination
// is on the preapproved whitelist.
//
// Lookup strategy (O(1) for the common case):
//  1. Direct map lookup for hostname-only entries.
//  2. For path-scoped entries, iterate the (small) prefix list and enforce
//     segment boundaries: the prefix "/anthropics" must not match the
//     pathname "/anthropics-evil".
func IsPreapprovedHost(hostname, pathname string) bool {
	// Fast path: plain hostname match.
	if _, ok := hostnameOnly[hostname]; ok {
		return true
	}

	// Slow path: path-prefix match.
	prefixes, ok := pathPrefixes[hostname]
	if !ok {
		return false
	}
	for _, p := range prefixes {
		// Exact match OR pathname starts with prefix followed by "/".
		// This enforces path-segment boundaries so that
		// "github.com/anthropics-evil" is never matched by the
		// "github.com/anthropics" rule.
		if pathname == p || strings.HasPrefix(pathname, p+"/") {
			return true
		}
	}
	return false
}

// IsPreapprovedURL is a convenience wrapper that parses rawURL and delegates
// to IsPreapprovedHost. Returns false on any parse error.
func IsPreapprovedURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return IsPreapprovedHost(u.Hostname(), u.EscapedPath())
}
