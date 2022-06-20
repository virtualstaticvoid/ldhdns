#!/bin/sh -e

indent() {
  sed -u 's/^/   /'
}

runtest() {
  echo "\n🔵 $@\n"
  exec "$@" 2>&1 | indent
}

echo "Running tests..."

runtest curl --ipv4 -v -sL http://web.ldh.dns
runtest curl --ipv6 -v -sL http://web.ldh.dns

runtest curl --ipv4 -v -sL http://web2.alt.dns
runtest curl --ipv6 -v -sL http://web2.alt.dns

runtest curl --ipv4 -v -sL http://api.ldh.dns
runtest curl --ipv6 -v -sL http://api.ldh.dns

runtest curl --ipv4 -v -sL http://api2.alt.dns
runtest curl --ipv6 -v -sL http://api2.alt.dns

runtest psql -h pgsql.ldh.dns -U postgres -c "select count(*) from information_schema.schemata;"
runtest psql -h pgsql2.alt.dns -U postgres -c "select count(*) from information_schema.schemata;"
