#!/bin/sh
# Daily backup of the database and the attached files, run by the backup container.
#   backup.sh loop    wait for migrations, then back up immediately and once a day
#   backup.sh once    back up now
#   backup.sh verify  restore the newest dump into a scratch database and check the newest files archive
# Each backup is verified the same way: a dump that does not restore is reported as failed.
set -u
# Warnings and errors only: "database ... does not exist, skipping" is no failure.
export PGOPTIONS='-c client_min_messages=warning'

DIR=/backups
CHECK_DB="${POSTGRES_BASE}_restore_check"
pg() { "$@" -h postgres -p "$POSTGRES_PORT" -U "$POSTGRES_USER"; }

migrations_ready() {
	pg psql -XqtA -v ON_ERROR_STOP=1 -v target="$1" -d "$POSTGRES_BASE" <<'SQL' | grep -qx t
SELECT to_regclass('public.schema_migrations') IS NOT NULL AS initialized \gset
\if :initialized
SELECT version >= :'target'::bigint AND NOT dirty FROM public.schema_migrations;
\endif
SQL
}

verify() {
	dump=$(ls -1 "$DIR"/db-*.dump 2>/dev/null | sort | tail -n 1)
	files=$(ls -1 "$DIR"/files-*.tar.gz 2>/dev/null | sort | tail -n 1)
	[ -n "$dump" ] || { echo "verify: no dump in $DIR"; return 1; }
	pg dropdb --if-exists "$CHECK_DB" || return 1
	pg createdb "$CHECK_DB" || return 1
	if ! pg pg_restore --no-owner --exit-on-error -d "$CHECK_DB" "$dump" >/dev/null; then
		pg dropdb --if-exists "$CHECK_DB"
		echo "verify: $dump does not restore"
		return 1
	fi
	counts=$(pg psql -X -tA -d "$CHECK_DB" -c "SELECT (SELECT count(*) FROM app.collection) || ' collections, ' || (SELECT count(*) FROM app.record) || ' records, ' || (SELECT count(*) FROM app.file) || ' files'")
	pg dropdb --if-exists "$CHECK_DB"
	[ -n "$counts" ] || { echo "verify: $dump restored without the app tables"; return 1; }
	if [ -n "$files" ] && ! tar -tzf "$files" >/dev/null; then
		echo "verify: $files is not a readable archive"
		return 1
	fi
	echo "verify: $(basename "$dump") restores: $counts"
}

once() {
	stamp=$(date +%Y%m%d-%H%M%S)
	if ! pg pg_dump -Fc -d "$POSTGRES_BASE" -f "$DIR/db-$stamp.part"; then
		rm -f "$DIR/db-$stamp.part"
		echo "backup $stamp FAILED: pg_dump"
		return 1
	fi
	mv "$DIR/db-$stamp.part" "$DIR/db-$stamp.dump"
	if ! tar -czf "$DIR/files-$stamp.part" -C /files .; then
		rm -f "$DIR/files-$stamp.part"
		echo "backup $stamp FAILED: files"
		return 1
	fi
	mv "$DIR/files-$stamp.part" "$DIR/files-$stamp.tar.gz"
	verify || { echo "backup $stamp FAILED: verify"; return 1; }
	find "$DIR" -type f -mtime +"$BACKUP_KEEP_DAYS" -delete
	touch "$DIR/.last-ok" # the container's healthcheck reads its age
	echo "backup $stamp done"
}

case "${1:-loop}" in
	once) once ;;
	verify) verify ;;
	loop)
		# The sequential migration filenames have zero-padded version numbers.
		for migration in /migrations/*.up.sql; do
			[ -f "$migration" ] || { echo "backup: no migrations found in /migrations" >&2; exit 1; }
			latest=${migration##*/}
		done
		target=${latest%%_*}
		echo "backup: waiting for database migration $target to finish"
		until migrations_ready "$target"; do sleep 5; done
		while true; do
			once
			sleep 86400
		done
		;;
	*) echo "usage: backup.sh loop|once|verify"; exit 2 ;;
esac
