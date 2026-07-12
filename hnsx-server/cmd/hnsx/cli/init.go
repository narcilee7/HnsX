package cli

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

//go:embed templates/*.yaml
var templatesFS embed.FS

// newInitCmd generates a new DomainSpec from a template.
//
// Deprecated: use `hnsx new <domain-id>` instead. `hnsx new` produces a
// per-domain folder (with README + .gitignore + next-step hints) on top of
// the same domain.yaml. `hnsx init` is preserved for backward compatibility
// and will be removed in v1.1.
func newInitCmd(cfg *Config) *cobra.Command {
	var (
		templateName string
		outputDir    string
		setVars      []string
	)

	cmd := &cobra.Command{
		Use:        "init [flags] [domain-id]",
		Short:      "Generate a new DomainSpec from a template (deprecated: use `hnsx new`)",
		Deprecated: "use `hnsx new <domain-id>` instead — it scaffolds a full Domain folder",
		Long: `Generate a new domain.yaml from a built-in template or the template index.

This command is deprecated. Run "hnsx new <domain-id>" instead to get a
per-domain folder (domain.yaml + README.md + .gitignore) with the same
template content.

Available built-in templates:
  blank              minimal single-agent domain
  customer-service   triage + billing workflow
  code-review        PR reviewer with self_check
  research-assistant ReAct agent with http tool

The template index (templates/index.yaml) can also be referenced by id.

Examples:
  hnsx init
  hnsx init my-service --template customer-service
  hnsx init my-service --template customer-service --set company_name=Acme --output-dir ./domains`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domainID := "my-domain"
			if len(args) > 0 {
				domainID = args[0]
			}

			vars := map[string]string{
				"domain_id":    domainID,
				"company_name": "Acme",
			}

			data, source, err := loadTemplate(cfg, templateName)
			if err != nil {
				return err
			}

			// Apply defaults from the template index, then user overrides.
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

			if _, err := spec.Parse([]byte(rendered)); err != nil {
				return fmt.Errorf("rendered domain is invalid: %w", err)
			}

			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("create output dir: %w", err)
			}
			outPath := filepath.Join(outputDir, "domain.yaml")
			if err := os.WriteFile(outPath, []byte(rendered), 0o644); err != nil {
				return fmt.Errorf("write domain.yaml: %w", err)
			}

			o := NewOutput(cfg.Output)
			if cfg.Output == "json" {
				o.Print(map[string]string{
					"path":     outPath,
					"template": templateName,
					"source":   source,
					"domain":   domainID,
				})
				return nil
			}
			o.Line("✓ created %s from template %q (%s)", outPath, templateName, source)
			return nil
		},
	}

	cmd.Flags().StringVar(&templateName, "template", "blank", "template to use (built-in or index id)")
	cmd.Flags().StringVar(&outputDir, "output-dir", ".", "directory to write domain.yaml")
	cmd.Flags().StringArrayVar(&setVars, "set", nil, "set template variables (key=value); can be repeated")

	return cmd
}

// loadTemplate returns template bytes and a source label. It first tries the
// embedded built-ins, then the template index, then gives up.
func loadTemplate(cfg *Config, name string) ([]byte, string, error) {
	if data, err := templatesFS.ReadFile("templates/" + name + ".yaml"); err == nil {
		return data, "built-in", nil
	}
	if entry := findTemplateEntry(cfg, name); entry != nil {
		path := filepath.Join(cfg.RepoRoot, entry.Source)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("read indexed template %q from %s: %w", name, path, err)
		}
		return data, "index", nil
	}
	return nil, "", fmt.Errorf("unknown template %q (run `hnsx examples` to list)", name)
}

func findTemplateEntry(cfg *Config, name string) *TemplateEntry {
	idx, err := loadTemplateIndex(cfg)
	if err != nil {
		return nil
	}
	for i := range idx.Templates {
		if idx.Templates[i].ID == name {
			return &idx.Templates[i]
		}
	}
	return nil
}

func renderTemplateVars(tmpl string, vars map[string]string) string {
	out := tmpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}
