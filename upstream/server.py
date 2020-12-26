  
from flask import Flask, request, abort

import uuid
import sys, os, getopt
import urllib.request
import urllib.parse

app = Flask(__name__)

uid = uuid.uuid4()

@app.route('/')
def index():
    print(request.headers)
    print("--------------")
    return str(uid)

@app.route('/healthz')
def health():
    print(request.headers)
    print("/healthz")
    return "ok"   

import sys, getopt

def main(argv):
   port = 8081
   try:
      opts, args = getopt.getopt(argv,"p:",["port="])
   except getopt.GetoptError:
      print('server.py -p <portnumber>')
      sys.exit(2)
   for opt, arg in opts:
      if opt in ("-p", "--port"):
         port = arg

   print("Registering endpoint: 127.0.0.1:", port)
   url = 'http://localhost:5000/edsservice/register?endpoint=127.0.0.1:' + str(port)
   f = urllib.request.urlopen(url)
   print(f.read().decode('utf-8'))

   print("Starting server with uuid: " + str(uid))
   app.run(host='0.0.0.0', port=int(port), debug=True)

if __name__ == "__main__":
   main(sys.argv[1:])


