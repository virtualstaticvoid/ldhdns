#!/usr/bin/dumb-init /bin/bash
set -e
# set -x # debug

# HACK: give docker daemon a chance to catch up
# occasionally the list of running containers doesn't include "us" yet
sleep 1

# run in controller mode
exec ldhdns controller --network-id "${LDHDNS_NETWORK_ID}" \
                       --domain-suffix "${LDHDNS_DOMAIN_SUFFIX}" \
                       --subdomain-label "${LDHDNS_SUBDOMAIN_LABEL}" \
                       --container-name "${LDHDNS_CONTAINER_NAME}"
