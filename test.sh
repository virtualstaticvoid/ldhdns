#!/bin/sh -e

echo "Running tests..."

curl --ipv4 -v http://web.ldh.dns
curl --ipv6 -v http://web.ldh.dns

curl --ipv4 -v http://web2.alt.dns
curl --ipv6 -v http://web2.alt.dns

psql -h pgsql.ldh.dns -U postgres -c 'select count(*) from information_schema.schemata;'
psql -h pgsql2.alt.dns -U postgres -c 'select count(*) from information_schema.schemata;'

echo "Tests completed successfully"
