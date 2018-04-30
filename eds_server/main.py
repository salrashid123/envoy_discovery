  
from flask import Flask, request, abort
from flask_restplus import Api, Resource, fields, marshal
from functools import wraps

import logging, json

app = Flask(__name__)

api = Api(app, version='v1', title='Envoy Service API',
    description='A simple service_name API'
)

parser = api.parser()

host = api.model('host', {
        "ip_address": fields.String(required=True, default="127.0.0.1", description='The ip_address'),
        "port": fields.Integer(required=True, default=18080, description='port'),
        'tags': fields.Nested(api.model('tags', {
            "az": fields.String(required=False, default='us-central1-a', description='zone (optional)'),
            "canary": fields.Boolean(required=False, default=False, description='canary status (optional)'),
            "load_balancing_weight": fields.Integer(required=False, default=50, description='load_balancing_weight (optional)')          
        }))  
})

services = api.model('service', {
    'hosts': fields.List(fields.Nested(host))
})

_VERSION = "v1"

class ServicesDAO(object):
    def __init__(self):
        self.services = {}

    def get(self, service_name):
        if (service_name in self.services):        
           return self.services[service_name]
        api.abort(404, "Services {} doesn't exist".format(service_name))

    def create(self,service_name,data):
        if (service_name in self.services):
            api.abort(409, "Serivce {} already exists".format(str(service_name)))
        self.services[service_name] = data
        return data

    def update(self, service_name, data):
        if (service_name in self.services):
          self.services[service_name] = data
          return data
        api.abort(404, "Service {} doesn't exist".format(service_name))

    def delete(self, service_name):
        if (service_name in self.services):
          del self.services[service_name]
          return
        api.abort(404, "Service {} doesn't exist".format(service_name))

DAO = ServicesDAO()


v2_request = api.model('req', {
  'node': fields.Nested(api.model("node",{
  'build_version': fields.String(required=True, description='...', default='fd44fd6051f5d1de3b020d0e03685c24075ba388/1.6.0-dev/Clean/RELEASE'),      
  'cluster': fields.String(required=True, description='...', default='mycluster'),  
  'id': fields.String(required=True, description='...', default='test-id')
  })
  ),
  "resource_names": fields.List(fields.String(default="myservice"))
})

@api.route('/v2/discovery:endpoints')
class Servicesv2(Resource):
    @api.doc('get_service_v2')
    @api.response(404, 'Service not found')
    @api.expect(v2_request)
    def post(self):
        '''Get hosts for service v2'''
        data = json.loads(request.data)
        print " Inbound v2 request for discovery.  POST payload: " + str(data)
        resp = ''
        try:
          id = data['node']["id"]
          cluster = data['node']["cluster"]
          resource_names = data["resource_names"]          
          for r in resource_names:
              if (DAO.services.has_key(r)):
                svc = DAO.services[r]
                endpoints =  []       
                for host in svc.get("hosts"):
                   endpoints.append( 
                       {"endpoint": {
                        "address":  {
                           "socket_address": {
                               "address": host.get("ip_address"),
                               "port_value":  host.get("port")
                           }
                        }
                       }}                                          
                   )
                resp = {
                  "version_info": _VERSION,
                  "resources": [ 
                      {
                        "@type":"type.googleapis.com/envoy.api.v2.ClusterLoadAssignment",
                        "cluster_name": r,
                        "endpoints": [
                            {
                                "lb_endpoints": endpoints
                            }
                        ]
                      }
                  ]
                }
              else:
                 resp = {
                  "version_info": _VERSION,
                  "resources": [ 
                      {
                      "@type":"type.googleapis.com/envoy.api.v2.ClusterLoadAssignment",
                      "cluster_name": r
                      }
                   ] 
                 }
              return resp
          api.abort(400, "Service Name not provided")        
        except KeyError: 
          api.abort(404, "Service doesn't exist")


@api.route('/v1/registration/<string:service_name>')
class Servicesv1(Resource):
    @api.doc('get_service_v1')
    @api.response(404, 'Service not found')
    @api.expect(parser)    
    @api.marshal_with(services)    
    def get(self, service_name):
        '''Get hosts for service v1'''
        print " Inbound v2 request for discovery.  GET service_name: " + service_name        
        try:
          return DAO.services[service_name]        
        except KeyError: 
          api.abort(404, "Service {} doesn't exist".format(service_name))

@api.route('/edsservice')
class ListServices(Resource):
    @api.doc('list_services')
    def get(self):
        '''List all resources'''        
        return DAO.services

@api.route('/edsservice/<string:service_name>')
class Services(Resource):
    @api.doc('get_service')
    @api.response(404, 'Service not found')
    @api.expect(parser)    
    @api.marshal_with(services)    
    def get(self, service_name):
        '''List hosts for service'''
        try:
          return DAO.services[service_name]        
        except KeyError: 
          api.abort(404, "Service {} doesn't exist".format(service_name))

    @api.doc('create_service')
    @api.marshal_with(services, code=201)
    @api.expect(services)
    @api.expect(parser)
    def post(self,service_name):
        '''Create a given resource'''        
        return DAO.create(service_name,api.payload), 201

    @api.doc('delete_service')
    @api.response(204, 'Serivice deleted')
    @api.expect(parser)    
    def delete(self, service_name):
        '''Delete a given resource'''
        DAO.delete(service_name)
        return '', 204

    @api.expect(services)
    @api.response(404, 'Service not found')
    @api.marshal_with(services)
    def put(self, service_name):
        '''Update a given resource'''        
        return DAO.update(service_name, api.payload)


app.run(host='0.0.0.0', port=8080, debug=True)
