#!/usr/bin/dumb-init /bin/bash
set -e
# set -x # debug

cmd=$1

case $cmd in

	controller)
		# run in controller mode
		exec ldhdns controller --network-id $LDHDNS_NETWORK_ID --domain-suffix $LDHDNS_DOMAIN_SUFFIX --subdomain-label $LDHDNS_SUBDOMAIN_LABEL
	;;

	dns)
		# delegate to s6 to run in DNS mode
		# runs "ldhdns dns" and "dnsmasq" services
		exec /init
	;;

  *)
    echo "ERROR: Unknown command '$cmd'"
    exit 1
  ;;

esac

exit 0
