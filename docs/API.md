##  /cluster/specs
* GET
* request body

    ```json
        [
          {
            "namespace": "namespace1",
            "deployments": [
              "deployment1"
            ],
            "statefulsets": [
              "statefulset1",
              "statefulset2"
            ],
            "services": [
              "service1"
            ]
          },
          {
            "namespace": "namespace2",
            "deployments": [
              "deployment1",
              "deployment2"
            ],
            "services": [
              "service1"
            ]
          }
        ]
    ```
 * response
    
    ```json
    [
      {
        "namespace": "namespace2",
        "deployments": [
          {
            "name": "deployment1",
            "k8s_spec": {
              "kind": "Deployment",
              "metadata": {},
              "spec": {},
              "status": {}
            },
             "pods" :[ "p1", "p2"]
          },
          {
            "name": "deployment2",
            "k8s_spec": {
              "kind": "Deployment",
              "metadata": {},
              "spec": {},
              "status": {}
            },
            "pods" :["p1", "p3"]
          }
        ],
        "services": [
          {
            "name": "service1",
            "k8s_spec": {
              "kind": "Service",
              "metadata": {},
              "spec": {},
              "status": {}
            },
           "pods" :["p1", "p3"]
          }
        ],
        "statefulsets": []
      }
    ]
    ```

##  /cluster/nodes
* GET
* request body
    ```json
    [
              {
                "namespace": "namespace1",
                "deployments": [
                  "deployment1"
                ],
                "statefulsets": [
                  "statefulset1",
                  "statefulset2"
                ],
                "services": [
                  "service1"
                ]
              },
              {
                "namespace": "namespace2",
                "deployments": [
                  "deployment1",
                  "deployment2"
                ],
                "services": [
                  "service1"
                ]
              }
            ]
    ```
* response
    ```json
    [
        "gke-tech-1510692653-default-pool-f0a5b843-2c8z",
        "gke-tech-1510692653-default-pool-f0a5b843-s8fb"
    ]
    ```

##  /cluster/mappings

* GET
* request body
    ```json
    [
      "services","deployments","statefulsets"
    ]
    ```

* response
    ```json
    [
      {
        "namespace": "namespace1",
        "deployments": ["d1","d2"],
        "services": ["s1","s23"],
        "statefulsets": ["ss"]
      },
      {
        "namespace": "namespace2",
        "deployment": ["d1","d4"],
        "service": ["s2","s4"],
        "statefulset": ["bb"]
      }
    ]
    ```

## /cluster/appmetrics

* GET

* request body
    ```json
    { 
       "k8sType": "deployment", 
       "name": "goddd", 
       "namespace": "default", 
       "prometheus": { 
           "metricPort": 8080 
       } 
    }

    ```
    
* response

    ```json
    { 
      "metrics": [ "app_goddd..." ] 
    }
    ```
