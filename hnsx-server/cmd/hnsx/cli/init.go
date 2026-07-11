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

// newInitCmd generates a new DomainSpec from a built-in template.
func newInitCmd(cfg *Config) *cobra.Command {
	var (
		templateName string
		outputDir    string
		setVars      []string
	)

	cmd := &cobra.Command{
		Use:   "init [flags] [domain-id]",
		Short: "Generate a new DomainSpec from a template",
		Long: `Generate a new domain.yaml from a built-in template.

Available templates:
  blank              minimal single-agent domain
  customer-service   triage + billing workflow
  code-review        PR reviewer with self_check
  research-assistant ReAct agent with http tool

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
			for _, s := range setVars {
				k, v, ok := strings.Cut(s, "=")
				if !ok {
					return fmt.Errorf("--set must be key=value: %q", s)
				}
				vars[k] = v
			}

			data, err := templatesFS.ReadFile("templates/" + templateName + ".yaml")
			if err != nil {
				return fmt.Errorf("unknown template %q (available: blank, customer-service, code-review, research-assistant)", templateName)
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
					"domain":   domainID,
				})
				return nil
			}
			o.Line("✓ created %s from template %q", outPath, templateName)
			return nil
		},
	}

	cmd.Flags().StringVar(&templateName, "template", "blank", "template to use (blank, customer-service, code-review, research-assistant)")
	cmd.Flags().StringVar(&outputDir, "output-dir", ".", "directory to write domain.yaml")
	cmd.Flags().StringArrayVar(&setVars, "set", nil, "set template variables (key=value); can be repeated")

	return cmd
}

func renderTemplateVars(tmpl string, vars map[string]string) string {
	out := tmpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}
