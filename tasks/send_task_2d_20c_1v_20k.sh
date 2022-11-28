curl -X POST					\
	http://100.27.4.24:8080/rpc		\
	-H 'cache-control: no-cache'		\
	-H 'content-type: application/json'	\
	-d '{
		"method": "JSONServer.Send_task",
		"params": [{
			"resource_link": "https://raw.githubusercontent.com/sgaragagghu/sdcc-clustering-datasets/master/sdcc/2d-20c.csv",
			"mappers_amount": 2,
			"margin": 10,
			"separate_entries": "\n",
			"separate_properties": ",",
			"properties_amount": 2,
			"initialization_algorithm": "clustering",
			"map_algorithm": "clustering",
			"map_algorithm_parameters": [20],
			"reducers_amount": 2,
			"reduce_algorithm": "clustering",
			"join_algorithm": "clustering",
			"iteration_algorithm": "clustering",
			"iteration_algorithm_parameters": [2]
		}],
		"id": "1"
	}'
