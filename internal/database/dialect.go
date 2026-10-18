package database

import "fmt"

// TrafficMax caps every traffic counter safely below math.MaxInt64 (~9.22e18)
// so that one more delta can never overflow int64. SQLite silently promotes an
// overflowing INTEGER to REAL, after which the column no longer scans into the
// Go int64 field and every reader of the table fails (#5762).
const TrafficMax = int64(9_000_000_000_000_000_000)

func ClampedAddExpr(col string) string {
	if IsPostgres() || IsMySQL() {
		return fmt.Sprintf("LEAST(%s + ?, %d)", col, TrafficMax)
	}
	return fmt.Sprintf("MIN(%s + ?, %d)", col, TrafficMax)
}

// JSONClientsFromInbound yields one row per settings.clients element, exposed
// as client.value for JSONFieldText.
func JSONClientsFromInbound() string {
	if IsPostgres() {
		return "FROM inbounds, jsonb_array_elements(inbounds.settings::jsonb -> 'clients') AS client(value)"
	}
	if IsMySQL() {
		return "FROM inbounds, JSON_TABLE(inbounds.settings, '$.clients[*]' COLUMNS (value JSON PATH '$')) AS client"
	}
	return "FROM inbounds, JSON_EACH(JSON_EXTRACT(inbounds.settings, '$.clients')) AS client"
}

func JSONFieldText(expr, key string) string {
	if IsPostgres() {
		return fmt.Sprintf("(%s ->> '%s')", expr, key)
	}
	if IsMySQL() {
		return fmt.Sprintf("JSON_VALUE(%s, '$.%s')", expr, key)
	}

	return fmt.Sprintf("TRIM(JSON_EXTRACT(%s, '$.%s'), '\"')", expr, key)
}

func GreatestExpr(a, b string) string {
	if IsPostgres() {
		return fmt.Sprintf("GREATEST(%s::bigint, %s::bigint)", a, b)
	}
	if IsMySQL() {
		return fmt.Sprintf("GREATEST(%s, %s)", a, b)
	}
	return fmt.Sprintf("MAX(%s, %s)", a, b)
}

// ClientTrafficEnableMergeExpr: placeholders nodeEnable, nodeExpiry, nodeTotal,
// now, deltaUp, deltaDown. Mirrors nodeDisableIsStale (#6228 / #4917).
func ClientTrafficEnableMergeExpr() string {
	if IsPostgres() {
		return `CASE
			WHEN ?::boolean THEN enable::boolean
			WHEN (expiry_time <> CAST(? AS BIGINT) OR total <> CAST(? AS BIGINT))
				AND (expiry_time <= 0 OR expiry_time > CAST(? AS BIGINT))
				AND (total <= 0 OR up + ? + down + ? < total) THEN enable::boolean
			ELSE false
		END`
	}
	return fmt.Sprintf(`CASE
		WHEN ? THEN enable
		WHEN (expiry_time <> CAST(? AS %[1]s) OR total <> CAST(? AS %[1]s))
			AND (expiry_time <= 0 OR expiry_time > CAST(? AS %[1]s))
			AND (total <= 0 OR up + ? + down + ? < total) THEN enable
		ELSE 0
	END`, bigintCastType())
}

// ClientTrafficExpiryMergeExpr: placeholder nodeExpiry once. Master absolute is
// kept; CAST avoids Postgres int4 inference on ms timestamps.
func ClientTrafficExpiryMergeExpr() string {
	return fmt.Sprintf(`CASE
		WHEN expiry_time > 0 THEN expiry_time
		ELSE CAST(? AS %s)
	END`, bigintCastType())
}

// CastTextExpr renders expr as text; MySQL has no CAST(... AS TEXT).
func CastTextExpr(expr string) string {
	if IsMySQL() {
		return fmt.Sprintf("CAST(%s AS CHAR)", expr)
	}
	return fmt.Sprintf("CAST(%s AS TEXT)", expr)
}

// bigintCastType is the CAST target for a 64-bit integer: MySQL rejects
// CAST(... AS BIGINT) and spells it SIGNED.
func bigintCastType() string {
	if IsMySQL() {
		return "SIGNED"
	}
	return "BIGINT"
}
