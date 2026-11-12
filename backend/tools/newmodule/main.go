// Command newmodule copies the products example into a new module: schema,
// domain, ports, use case, repository, handler, tests and web screens, with the
// wiring lines added next to the products ones. Run it through `make module`.
//
//	go run ./tools/newmodule -name invoice [-plural invoices]
//
// It edits files in place; review the diff, then replace the copied fields
// (name, price, enabled) with the module's own.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// shortcut: one lowercase word; names like invoice_item need their identifiers fixed by hand.
var nameFormat = regexp.MustCompile(`^[a-z]{2,30}$`)

// SQL keywords that cannot name an unquoted table, and names the template already uses.
var reserved = []string{
	"all", "and", "any", "case", "check", "column", "create", "default", "desc", "do", "end", "for",
	"from", "grant", "group", "having", "in", "into", "join", "limit", "not", "null", "offset",
	"on", "or", "order", "select", "table", "to", "union", "user", "where", "with",
	"account", "admin", "audit", "auth", "health", "job", "jobs", "mailer", "notification",
	"permission", "preference", "product", "role", "session", "users", "roles", "products",
}

type names struct{ s, p, S, P string } // singular, plural, and their Title forms

func (n names) apply(text string) string {
	return strings.NewReplacer(
		"Products", n.P, "products", n.p, "Product", n.S, "product", n.s,
		"Produtos", n.P, "produtos", n.p, "Produto", n.S, "produto", n.s,
	).Replace(text)
}

func main() {
	name := flag.String("name", "", "singular, lowercase: invoice")
	plural := flag.String("plural", "", "plural, lowercase (default: name + s)")
	root := flag.String("root", "..", "repository root, holding backend/ and frontend/")
	flag.Parse()
	if *plural == "" {
		*plural = *name + "s"
	}
	if err := run(*root, *name, *plural); err != nil {
		fmt.Fprintln(os.Stderr, "newmodule:", err)
		os.Exit(1)
	}
}

func run(root, name, plural string) error {
	for _, v := range []string{name, plural} {
		if !nameFormat.MatchString(v) {
			return fmt.Errorf("%q must be 2 to 30 lowercase letters", v)
		}
		if slices.Contains(reserved, v) {
			return fmt.Errorf("%q is reserved by SQL or by the template", v)
		}
	}
	if name == plural {
		return errors.New("name and plural must differ")
	}
	n := names{s: name, p: plural, S: strings.ToUpper(name[:1]) + name[1:], P: strings.ToUpper(plural[:1]) + plural[1:]}
	b := filepath.Join(root, "backend")
	f := filepath.Join(root, "frontend")
	if _, err := os.Stat(filepath.Join(b, "internal/core/usecase", name)); err == nil {
		return fmt.Errorf("module %q already exists", name)
	}

	// Whole files and folders, copied with every name replaced.
	copies := [][2]string{
		{"backend/internal/core/domain/product.go", "backend/internal/core/domain/" + name + ".go"},
		{"backend/internal/core/port/input/product.go", "backend/internal/core/port/input/" + name + ".go"},
		{"backend/internal/core/port/output/product.go", "backend/internal/core/port/output/" + name + ".go"},
		{"backend/internal/core/usecase/product", "backend/internal/core/usecase/" + name},
		{"backend/internal/adapter/outbound/persistence/postgres/product", "backend/internal/adapter/outbound/persistence/postgres/" + name},
		{"backend/internal/adapter/inbound/http/rest/handler/product", "backend/internal/adapter/inbound/http/rest/handler/" + name},
		{"backend/tests/integration/product_test.go", "backend/tests/integration/" + name + "_test.go"},
		{"frontend/src/app/(app)/products", "frontend/src/app/(app)/" + plural},
	}
	for _, c := range copies {
		if err := copyTree(filepath.Join(root, c[0]), filepath.Join(root, c[1]), n); err != nil {
			return err
		}
	}
	if err := migration(b, n); err != nil {
		return err
	}

	// Wiring: each line holding an anchor is repeated below itself with the names replaced.
	lines := map[string][]string{
		"backend/internal/core/port/input/application.go":                 {"ProductService"},
		"backend/internal/core/port/output/tx.go":                         {"Products() ProductRepository"},
		"backend/internal/core/domain/action.go":                          {`ResourceProducts = "products"`},
		"backend/internal/composition/wire.go":                            {`/usecase/product"`, "product.Jobs(log)", "product.New(store.Products(), store)"},
		"backend/internal/adapter/inbound/http/rest/server.go":            {`handler/product"`, "producthttp.New(app.Products)"},
		"backend/internal/adapter/outbound/persistence/postgres/store.go": {`/postgres/product"`, "func (s *Store) Products()"},
		"frontend/src/lib/navigation.ts":                                  {`href: "/products"`},
		"frontend/src/lib/validation.ts":                                  {"export const productName"},
		"frontend/src/lib/api/endpoints.ts":                               {"  Product,"},
		"frontend/src/lib/audit.ts":                                       {`r.action.startsWith("product.")`},
	}
	for file, anchors := range lines {
		if err := repeatLines(filepath.Join(root, file), anchors, n); err != nil {
			return err
		}
	}
	if err := repeatMessages(filepath.Join(f, "src/lib/i18n/messages.ts"), n); err != nil {
		return err
	}
	if err := edit(filepath.Join(f, "src/app/(app)/admin/audit/page.tsx"), `"product.", `, `"product.", "`+name+`.", `); err != nil {
		return err
	}
	// Blocks: the Product type, its input and the endpoint object.
	if err := repeatBlock(filepath.Join(f, "src/lib/api/types.ts"), "export interface Product {", "\n}\n", n); err != nil {
		return err
	}
	for _, start := range []string{"export interface ProductInput {", "export const products = {"} {
		end := "\n}\n"
		if strings.HasPrefix(start, "export const") {
			end = "\n};\n"
		}
		if err := repeatBlock(filepath.Join(f, "src/lib/api/endpoints.ts"), start, end, n); err != nil {
			return err
		}
	}

	fmt.Printf(`Module %q created next to products.
Left for you:
  - replace the copied fields (name, price, enabled) in the migration, domain, repository, handler, tests and screens;
  - review the pt-BR texts in frontend/src/lib/i18n/messages.ts (copied from products with the English name);
  - optional: a card on the home page (app/(app)/page.tsx) and a search in components/command-palette.tsx;
  - grant the new resource %q to a role under Admin > Roles.
`, name, plural)
	return nil
}

func copyTree(src, dst string, n names) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, n.apply(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(path) //nolint:gosec // a template path inside the repository
		if err != nil {
			return err
		}
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("%s already exists", target)
		}
		return os.WriteFile(target, []byte(n.apply(string(data))), 0o644) //nolint:gosec // repository source files; names are checked and paths come from the template
	})
}

// migration writes the next numbered pair from the products schema.
func migration(backend string, n names) error {
	dir := filepath.Join(backend, "deploy/db/migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	last := 0
	for _, e := range entries {
		var num int
		if _, err := fmt.Sscanf(e.Name(), "%06d_", &num); err == nil && num > last {
			last = num
		}
	}
	base := filepath.Join(dir, fmt.Sprintf("%06d_%s", last+1, n.p))
	for _, suffix := range []string{".up.sql", ".down.sql"} {
		data, err := os.ReadFile(filepath.Join(dir, "000004_products"+suffix)) //nolint:gosec // a template path inside the repository
		if err != nil {
			return err
		}
		if err := os.WriteFile(base+suffix, []byte(n.apply(string(data))), 0o644); err != nil { //nolint:gosec // repository source files; names are checked and paths come from the template
			return err
		}
	}
	return nil
}

func repeatLines(path string, anchors []string, n names) error {
	data, err := os.ReadFile(path) //nolint:gosec // a template path inside the repository
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(data), "\n")
	var out strings.Builder
	found := map[string]bool{}
	for _, line := range lines {
		out.WriteString(line)
		for _, a := range anchors {
			if strings.Contains(line, a) && !found[a] {
				found[a] = true
				out.WriteString(n.apply(line))
			}
		}
	}
	for _, a := range anchors {
		if !found[a] {
			return fmt.Errorf("%s: anchor %q not found; wire this line by hand", path, a)
		}
	}
	return os.WriteFile(path, []byte(out.String()), 0o644) //nolint:gosec // repository source files; names are checked and paths come from the template
}

var messageKey = regexp.MustCompile(`^\s+"(products\.|nav\.products"|resource\.products|audit\.action\.product\.|audit\.kind\.product"|error\.msg\.product_name")`)

func repeatMessages(path string, n names) error {
	data, err := os.ReadFile(path) //nolint:gosec // a template path inside the repository
	if err != nil {
		return err
	}
	var out strings.Builder
	for _, line := range strings.SplitAfter(string(data), "\n") {
		out.WriteString(line)
		if messageKey.MatchString(line) {
			out.WriteString(n.apply(line))
		}
	}
	return os.WriteFile(path, []byte(out.String()), 0o644) //nolint:gosec // repository source files; names are checked and paths come from the template
}

func repeatBlock(path, start, end string, n names) error {
	data, err := os.ReadFile(path) //nolint:gosec // a template path inside the repository
	if err != nil {
		return err
	}
	text := string(data)
	i := strings.Index(text, start)
	if i < 0 {
		return fmt.Errorf("%s: %q not found", path, start)
	}
	j := strings.Index(text[i:], end)
	if j < 0 {
		return fmt.Errorf("%s: end of %q not found", path, start)
	}
	block := text[i : i+j+len(end)]
	return os.WriteFile(path, []byte(strings.Replace(text, block, block+"\n"+n.apply(block), 1)), 0o644) //nolint:gosec // repository source files; names are checked and paths come from the template
}

func edit(path, old, replacement string) error {
	data, err := os.ReadFile(path) //nolint:gosec // a template path inside the repository
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), old) {
		return fmt.Errorf("%s: %q not found", path, old)
	}
	return os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o644) //nolint:gosec // repository source files; names are checked and paths come from the template
}
