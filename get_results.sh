curl -X POST \
   http://172.18.0.254:8080/rpc \
   -H 'cache-control: no-cache' \
   -H 'content-type: application/json' \
   -d '{
   "method": "JSONServer.Get_results",
   "params": [{}],"id": "1"}'
