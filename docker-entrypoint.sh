#!/usr/bin/dumb-init /bin/bash
set -e
# set -x # debug

cmd=$1

case $cmd in

	controller)
		# run in controller mode
		ldhdns controller --network-id $LDHDNS_NETWORK_ID --domain-suffix $LDHDNS_DOMAIN_SUFFIX
	;;

	dns)
		# delegate to supervisord to run in DNS mode
		# runs "ldhdns dns" and "dnsmasq" services
		supervisord --nodaemon --configuration /etc/ldhdns/supervisor/supervisord.conf
	;;

  *)
    echo "ERROR: Unknown command '$cmd'"
    exit 1
  ;;

esac

exit 0
