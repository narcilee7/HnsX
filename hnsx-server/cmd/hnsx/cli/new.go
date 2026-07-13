package cli

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

//go:embed assets/new/README.md.tmpl assets/new/.gitignore.tmpl
var newAssetsFS embed.FS

// newNewCmd generates a new Domain directory from a built-in template.
//
// Unlike `hnsx init`, which writes a single domain.yaml into --output-dir,
// `hnsx new` creates a per-domain folder:
//
//	hnsx new my-cs --template customer-service
//	  → ./my-cs/domain.yaml
//	  → ./my-cs/README.md   (next-steps + variable reference)
//	  → ./my-cs/.gitignore  (secrets, eval cache, runtime artifacts)
//
// `hnsx init` is preserved as a deprecated alias that prints a redirect
// warning. See init.go.
func newNewCmd(cfg *Config) *cobra.Command {
	var (
		templateName string
		outputDir    string
		setVars      []string
		listOnly     bool
		force        bool
	)

	cmd := &cobra.Command{
		Use:   "new <domain-id> [flags]",
		Short: "Scaffold a new Domain from a built-in template",
		Long: `Create a new Domain directory containing domain.yaml, README.md,
and .gitignore. Use 'hnsx new --list' to see available templates.

Available built-in templates:
  blank              minimal single-agent domain (good first template)
  customer-service   triage + billing workflow
  code-review        PR reviewer with self_check
  research-assistant ReAct agent with http tool

The template index (templates/index.yaml) can also be referenced by id.

Examples:
  hnsx new my-service
  hnsx new my-service --template customer-service
  hnsx new my-service --template customer-service --set company_name=Acme
  hnsx new my-service --template customer-service --output-dir ./domains
  hnsx new --list`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if listOnly {
				return runNewList(cfg, NewOutputWriter(cfg.Output, cmd.OutOrStdout()))
			}

			domainID := "my-domain"
			if len(args) > 0 {
				domainID = args[0]
			}
			if domainID == "" {
				return fmt.Errorf("domain id is required (or pass --list to see templates)")
			}
			if !isSafeDomainID(domainID) {
				return fmt.Errorf("invalid domain id %q (use lowercase letters, digits, '-' or '_'; must start with a letter)", domainID)
			}

			target := filepath.Join(outputDir, domainID)
			if !force {
				if _, err := os.Stat(target); err == nil {
					return fmt.Errorf("target directory already exists: %s (use --force to overwrite)", target)
				} else if !os.IsNotExist(err) {
					return fmt.Errorf("check target directory: %w", err)
				}
			}

			vars := map[string]string{
				"domain_id":    domainID,
				"company_name": "Acme",
				"template":     templateName,
			}

			data, source, err := loadTemplate(cfg, templateName)
			if err != nil {
				return err
			}

			if entry := findTemplateEntry(cfg, templateName); entry != nil {
				for _, v := range entry.Variables {
					if v.Name != "" {
						vars[v.Name] = v.Default
					}
				}
			}
			for _, s := range setVars {
				k, v, ok := strings.Cut(s, "=")
				if !ok {
					return fmt.Errorf("--set must be key=value: %q", s)
				}
				vars[k] = v
			}

			rendered := renderTemplateVars(string(data), vars)
			if _, err := domain.Parse([]byte(rendered)); err != nil {
				return fmt.Errorf("rendered domain is invalid: %w", err)
			}

			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create target directory: %w", err)
			}

			// domain.yaml
			if err := os.WriteFile(filepath.Join(target, "domain.yaml"), []byte(rendered), 0o644); err != nil {
				return fmt.Errorf("write domain.yaml: %w", err)
			}
			// README.md (per-domain next-steps)
			readme := renderTemplateVars(loadNewAsset("assets/new/README.md.tmpl"), vars)
			if err := os.WriteFile(filepath.Join(target, "README.md"), []byte(readme), 0o644); err != nil {
				return fmt.Errorf("write README.md: %w", err)
			}
			// .gitignore (scoped to the domain folder)
			gitignore := renderTemplateVars(loadNewAsset("assets/new/.gitignore.tmpl"), vars)
			if err := os.WriteFile(filepath.Join(target, ".gitignore"), []byte(gitignore), 0o644); err != nil {
				return fmt.Errorf("write .gitignore: %w", err)
			}

			o := NewOutputWriter(cfg.Output, cmd.OutOrStdout())
			if cfg.Output == "json" {
				o.Print(map[string]any{
					"path":     target,
					"template": templateName,
					"source":   source,
					"domain":   domainID,
					"files":    []string{"domain.yaml", "README.md", ".gitignore"},
				})
				return nil
			}

			o.Line("✓ Scaffolded %s from template %q (%s)", target, templateName, source)
			o.Line("")
			o.Line("  %s/domain.yaml    DomainSpec", filepath.Base(target))
			o.Line("  %s/README.md      next steps + variable reference", filepath.Base(target))
			o.Line("  %s/.gitignore     runtime artifacts", filepath.Base(target))
			o.Line("")
			o.Line("Next:")
			o.Line("  cd %s", target)
			o.Line("  hnsx validate domain.yaml")
			o.Line("  hnsx run --domain domain.yaml --trigger '{\"question\":\"hi\"}'")
			o.Line("  # or to register against a running server:")
			o.Line("  hnsx domain apply --file domain.yaml")
			return nil
		},
	}

	cmd.Flags().StringVar(&templateName, "template", "blank", "template to use (built-in or index id)")
	cmd.Flags().StringVar(&outputDir, "output-dir", ".", "parent directory in which to create <domain-id>/")
	cmd.Flags().StringArrayVar(&setVars, "set", nil, "set template variables (key=value); can be repeated")
	cmd.Flags().BoolVar(&listOnly, "list", false, "list available templates and exit")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing target directory")

	return cmd
}

func runNewList(cfg *Config, out *Output) error {
	builtins := listBuiltinTemplates()
	indexed := listIndexedTemplates(cfg)

	if cfg.Output == "json" {
		out.Print(map[string]any{
			"built_in": builtins,
			"index":    indexed,
		})
		return nil
	}

	out.Line("Built-in templates:")
	for _, t := range builtins {
		out.Line("  %-20s %s", t.ID, t.Desc)
	}
	if len(indexed) > 0 {
		out.Line("")
		out.Line("From template index (templates/index.yaml):")
		for _, t := range indexed {
			out.Line("  %-20s %s", t.ID, t.Desc)
		}
	}
	out.Line("")
	out.Line("Use: hnsx new <domain-id> --template <id>")
	return nil
}

type newTemplateInfo struct {
	ID   string `json:"id"`
	Desc string `json:"desc"`
}

func listBuiltinTemplates() []newTemplateInfo {
	// Mirrors the docblock on `newNewCmd` — keep in sync if you add a
	// built-in. We don't read .yaml files here because their shape varies;
	// the docblock is the contract.
	return []newTemplateInfo{
		{ID: "blank", Desc: "minimal single-agent domain (good first template)"},
		{ID: "customer-service", Desc: "triage + billing workflow"},
		{ID: "code-review", Desc: "PR reviewer with self_check"},
		{ID: "research-assistant", Desc: "ReAct agent with http tool"},
	}
}

func listIndexedTemplates(cfg *Config) []newTemplateInfo {
	idx, err := loadTemplateIndex(cfg)
	if err != nil {
		return nil
	}
	out := make([]newTemplateInfo, 0, len(idx.Templates))
	seen := map[string]bool{}
	for _, t := range listBuiltinTemplates() {
		seen[t.ID] = true
	}
	for _, e := range idx.Templates {
		if seen[e.ID] {
			continue
		}
		out = append(out, newTemplateInfo{ID: e.ID, Desc: e.Description})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// isSafeDomainID prevents the user from creating directories that escape
// the working tree (e.g. "../../etc/passwd") via the <domain-id> arg.
func isSafeDomainID(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.ContainsAny(s, "/\\") {
		return false
	}
	if s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// loadNewAsset returns the bytes of an embedded asset under assets/new/.
// Falls back to a clear error if the embed is missing — that means a
// template was renamed but the asset wasn't.
func loadNewAsset(path string) string {
	data, err := newAssetsFS.ReadFile(path)
	if err != nil {
		// We panic here because the embed is shipped with the binary; a
		// missing asset is a build-time bug, not a runtime condition.
		var _ fs.File = nil // keep imports honest across refactors
		panic(fmt.Sprintf("hnsx binary missing embedded asset %q: %v", path, err))
	}
	return string(data)
}