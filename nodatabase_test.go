package paymentexpiry_test

import (
	"os/exec"
	"strings"
	"testing"
)

// databasePackages are the prefixes of every driver or client that opens a database connection.
var databasePackages = []string{
	"database/sql",
	"github.com/jackc/pgx",
	"github.com/lib/pq",
	"github.com/go-sql-driver",
	"go.mongodb.org/mongo-driver",
}

// databaseImports returns the packages of deps that open a database connection.
func databaseImports(deps []string) []string {
	var found []string
	for _, dep := range deps {
		for _, prefix := range databasePackages {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				found = append(found, dep)
			}
		}
	}
	return found
}

func TestC_the_detector_finds_a_driver_among_the_dependencies(t *testing.T) {
	deps := []string{"fmt", "net/http", "github.com/jackc/pgx/v5/pgxpool", "database/sql", "github.com/example/other"}
	if got := databaseImports(deps); len(got) != 2 {
		t.Fatalf("detector found %v, want pgx and database/sql", got)
	}
	if got := databaseImports([]string{"fmt", "net/http", "database/sqlx-not-a-driver-here"}); len(got) != 0 {
		t.Fatalf("detector flagged %v, a look-alike name is not a driver", got)
	}
}

// AC-WK-08.9: the worker has no database connection; it reaches commercial-reservation only through its internal routes.
func TestC_the_worker_imports_no_database_driver(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "./...").Output()
	if err != nil {
		t.Fatalf("go list -deps ./...: %v", err)
	}
	if found := databaseImports(strings.Fields(string(out))); len(found) != 0 {
		t.Fatalf("the worker imports database packages %v, but it may call only internal routes (Annex J, J.3.2)", found)
	}
}
