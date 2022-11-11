curl -X POST					\
	http://172.18.0.254:8080/rpc		\
	-H 'cache-control: no-cache'		\
	-H 'content-type: application/json'	\
	-d '{
		"method": "JSONServer.Send_task",
		"params": [{
			"resource_link": "https://raw.githubusercontent.com/sgaragagghu/sdcc-clustering-datasets/master/sdcc/2d-4c.csv",
			"mappers_amount": 2,
			"margin": 10,
			"separate_entries": "\n",
			"separate_properties": ",",
			"properties_amount": 2,
			"initialization_algorithm": "clustering",
			"map_algorithm": "clustering",
			"map_algorithm_parameters": [4],
			"reducers_amount": 2,
			"iteration_algorithm": "clustering",
			"iteration_algorithm_parameters": [2]
		}],
		"id": "1"
	}'
