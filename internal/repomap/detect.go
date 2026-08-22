package repomap

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var manifestNames = map[string]bool{
	"go.mod": true, "go.work": true, "package.json": true,
	"package-lock.json": true, "npm-shrinkwrap.json": true, "pnpm-lock.yaml": true,
	"yarn.lock": true, "bun.lock": true, "bun.lockb": true,
	"pyproject.toml": true, "pytest.ini": true, "requirements.txt": true,
	"poetry.lock": true, "Pipfile": true, "uv.lock": true,
	"Cargo.toml": true, "Cargo.lock": true, "Makefile": true, "makefile": true,
	"justfile": true, "Justfile": true, "pom.xml": true,
	"build.gradle": true, "build.gradle.kts": true, "settings.gradle": true,
	"settings.gradle.kts": true, "Gemfile": true, "composer.json": true,
	"CMakeLists.txt": true, "Dockerfile": true,
}

var languageByExt = map[string]string{
	".go": "Go", ".js": "JavaScript", ".jsx": "JavaScript",
	".mjs": "JavaScript", ".cjs": "JavaScript", ".ts": "TypeScript",
	".tsx": "TypeScript", ".py": "Python", ".rs": "Rust",
	".rb": "Ruby", ".php": "PHP", ".java": "Java", ".kt": "Kotlin",
	".kts": "Kotlin", ".c": "C", ".h": "C", ".cc": "C++", ".cpp": "C++",
	".cs": "C#", ".swift": "Swift", ".scala": "Scala", ".ex": "Elixir",
	".exs": "Elixir", ".sh": "Shell", ".bash": "Shell", ".zsh": "Shell",
}

func detectFiles(root string, files []string, m *Map) {
	languages := map[string]bool{}
	frameworks := map[string]bool{}
	managers := map[string]bool{}
	sourceRoots := map[string]bool{}
	testRoots := map[string]bool{}
	manifests := map[string]bool{}
	rules := map[string]bool{}

	for _, file := range files {
		base := filepath.Base(file)
		ext := strings.ToLower(filepath.Ext(file))
		if language := languageByExt[ext]; language != "" {
			languages[language] = true
		}
		if manifestNames[base] {
			manifests[file] = true
		}
		if isRuleFile(file) {
			rules[file] = true
		}
		parts := strings.Split(filepath.ToSlash(file), "/")
		if len(parts) > 1 {
			if isSourceRoot(parts[0]) {
				sourceRoots[parts[0]] = true
			}
			if isTestRoot(parts[0]) {
				testRoots[parts[0]] = true
			}
		}
		if strings.HasSuffix(file, "_test.go") || strings.HasSuffix(file, "_test.py") ||
			strings.HasSuffix(file, ".test.js") || strings.HasSuffix(file, ".test.ts") ||
			strings.HasSuffix(file, ".spec.js") || strings.HasSuffix(file, ".spec.ts") {
			dir := filepath.ToSlash(filepath.Dir(file))
			if dir != "." {
				testRoots[dir] = true
			}
		}
	}

	if manifests["go.mod"] || manifests["go.work"] {
		languages["Go"] = true
		managers["go"] = true
		m.Commands.Build = append(m.Commands.Build, "go build ./...")
		m.Commands.Test = append(m.Commands.Test, "go test ./...")
		m.Commands.Lint = append(m.Commands.Lint, "go vet ./...")
		detectTextFrameworks(filepath.Join(root, "go.mod"), frameworks, map[string]string{
			"github.com/gin-gonic/gin": "Gin", "github.com/labstack/echo": "Echo",
			"github.com/gofiber/fiber": "Fiber", "github.com/charmbracelet/bubbletea": "Bubble Tea",
		})
	}
	if manifests["package.json"] {
		detectPackageJSON(filepath.Join(root, "package.json"), manifests, managers, frameworks, &m.Commands)
	}
	if manifests["pyproject.toml"] || manifests["requirements.txt"] || manifests["pytest.ini"] {
		languages["Python"] = true
		switch {
		case manifests["uv.lock"]:
			managers["uv"] = true
		case manifests["poetry.lock"]:
			managers["poetry"] = true
		default:
			managers["pip"] = true
		}
		m.Commands.Test = append(m.Commands.Test, "pytest -q")
		detectTextFrameworks(filepath.Join(root, "pyproject.toml"), frameworks, map[string]string{
			"django": "Django", "fastapi": "FastAPI", "flask": "Flask",
		})
		detectTextFrameworks(filepath.Join(root, "requirements.txt"), frameworks, map[string]string{
			"django": "Django", "fastapi": "FastAPI", "flask": "Flask",
		})
	}
	if manifests["Cargo.toml"] {
		languages["Rust"] = true
		managers["cargo"] = true
		m.Commands.Build = append(m.Commands.Build, "cargo build")
		m.Commands.Test = append(m.Commands.Test, "cargo test")
		m.Commands.Lint = append(m.Commands.Lint, "cargo clippy --all-targets --all-features")
		detectTextFrameworks(filepath.Join(root, "Cargo.toml"), frameworks, map[string]string{
			"tauri": "Tauri", "axum": "Axum", "actix-web": "Actix Web",
		})
	}
	if manifests["pom.xml"] {
		languages["Java"] = true
		managers["maven"] = true
		m.Commands.Build = append(m.Commands.Build, "mvn package")
		m.Commands.Test = append(m.Commands.Test, "mvn test")
	}
	if manifests["build.gradle"] || manifests["build.gradle.kts"] {
		managers["gradle"] = true
		m.Commands.Build = append(m.Commands.Build, "./gradlew build")
		m.Commands.Test = append(m.Commands.Test, "./gradlew test")
	}
	if manifests["Makefile"] || manifests["makefile"] {
		detectMakefile(filepath.Join(root, choose(manifests["Makefile"], "Makefile", "makefile")), &m.Commands)
	}

	m.Languages = keys(languages)
	m.Frameworks = keys(frameworks)
	m.PackageManagers = keys(managers)
	m.Manifests = keys(manifests)
	m.SourceRoots = keys(sourceRoots)
	m.TestRoots = keys(testRoots)
	m.Rules = keys(rules)
	m.Commands.Build = sortedUnique(m.Commands.Build)
	m.Commands.Test = sortedUnique(m.Commands.Test)
	m.Commands.Lint = sortedUnique(m.Commands.Lint)
}

func detectPackageJSON(path string, manifests map[string]bool, managers, frameworks map[string]bool, commands *Commands) {
	data, err := readSmall(path)
	if err != nil {
		return
	}
	var pkg struct {
		PackageManager string            `json:"packageManager"`
		Scripts        map[string]string `json:"scripts"`
		Dependencies   map[string]string `json:"dependencies"`
		DevDeps        map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return
	}
	manager := "npm"
	switch {
	case strings.HasPrefix(pkg.PackageManager, "pnpm@") || manifests["pnpm-lock.yaml"]:
		manager = "pnpm"
	case strings.HasPrefix(pkg.PackageManager, "yarn@") || manifests["yarn.lock"]:
		manager = "yarn"
	case strings.HasPrefix(pkg.PackageManager, "bun@") || manifests["bun.lock"] || manifests["bun.lockb"]:
		manager = "bun"
	}
	managers[manager] = true
	if pkg.Scripts["build"] != "" {
		commands.Build = append(commands.Build, manager+" run build")
	}
	if pkg.Scripts["test"] != "" {
		if manager == "npm" {
			commands.Test = append(commands.Test, "npm test --silent")
		} else {
			commands.Test = append(commands.Test, manager+" test")
		}
	}
	if pkg.Scripts["lint"] != "" {
		commands.Lint = append(commands.Lint, manager+" run lint")
	}
	deps := make(map[string]string, len(pkg.Dependencies)+len(pkg.DevDeps))
	for name, version := range pkg.Dependencies {
		deps[name] = version
	}
	for name, version := range pkg.DevDeps {
		deps[name] = version
	}
	known := map[string]string{
		"next": "Next.js", "react": "React", "vue": "Vue", "nuxt": "Nuxt",
		"svelte": "Svelte", "@sveltejs/kit": "SvelteKit", "astro": "Astro",
		"@angular/core": "Angular", "electron": "Electron", "vite": "Vite",
		"express": "Express", "fastify": "Fastify", "hono": "Hono",
	}
	for dependency, framework := range known {
		if _, ok := deps[dependency]; ok {
			frameworks[framework] = true
		}
	}
}

var makeTarget = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*:`)

func detectMakefile(path string, commands *Commands) {
	data, err := readSmall(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		match := makeTarget.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		switch match[1] {
		case "build":
			commands.Build = append(commands.Build, "make build")
		case "test", "check":
			commands.Test = append(commands.Test, "make "+match[1])
		case "lint", "vet":
			commands.Lint = append(commands.Lint, "make "+match[1])
		}
	}
}

func detectTextFrameworks(path string, frameworks map[string]bool, known map[string]string) {
	data, err := readSmall(path)
	if err != nil {
		return
	}
	text := strings.ToLower(string(data))
	for needle, framework := range known {
		if strings.Contains(text, strings.ToLower(needle)) {
			frameworks[framework] = true
		}
	}
}

func readSmall(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 1<<20))
}

func isRuleFile(file string) bool {
	file = filepath.ToSlash(file)
	base := filepath.Base(file)
	if base == "AGENTS.md" || base == "CLAUDE.md" || base == "GEMINI.md" || base == ".cursorrules" {
		return true
	}
	return file == ".github/copilot-instructions.md" || strings.HasPrefix(file, ".cursor/rules/")
}

func isSourceRoot(name string) bool {
	switch name {
	case "src", "lib", "app", "cmd", "internal", "pkg", "packages", "crates":
		return true
	default:
		return false
	}
}

func isTestRoot(name string) bool {
	switch name {
	case "test", "tests", "spec", "e2e", "__tests__":
		return true
	default:
		return false
	}
}

func keys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value, include := range values {
		if include {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	return compact(values)
}

func choose(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}
