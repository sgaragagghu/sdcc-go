curl -X POST					\
	http://54.160.85.128:8080/rpc		\
	-H 'cache-control: no-cache'		\
	-H 'content-type: application/json'	\
	-d '{
		"method": "JSONServer.Send_task",
		"params": [{
			"resource_link": "https://raw.githubusercontent.com/sgaragagghu/sdcc-clustering-datasets/master/sdcc/2d-4c-v4-100MB.csv",
			"mappers_amount": 4,
			"margin": 10,
			"separate_entries": "\n",
			"separate_properties": ",",
			"properties_amount": 2,
			"initialization_algorithm": "clustering",
			"map_algorithm": "clustering",
			"map_algorithm_parameters": [4],
			"reducers_amount": 4,
			"reduce_algorithm": "clustering",
			"join_algorithm": "clustering",
			"iteration_algorithm": "clustering",
			"iteration_algorithm_parameters": [2]
		}],
		"id": "1"
	}'
