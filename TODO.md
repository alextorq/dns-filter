# TODO

## Deployment
boost allow domain function with batch processing
Add deploy script for server
it must be easy to deploy the server create new influx db and organization

## Web UI
- Big button for stop filtering for several minutes
- Opportunity to add domain to block list from web UI
- Add default user/password for web UI
- Add ability to change user/password for web UI


## Other tasks
- Add more logging
- Add tests
- Add loki support for saving logs
- Expand `clients/discovery/oui.txt` — current ~80 curated entries miss many home-LAN vendors; consider embedding a larger subset of IEEE OUI (or a build-time fetch of Wireshark's manuf) so the Network scan vendor column populates more often
