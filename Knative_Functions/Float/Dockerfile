FROM python:3.8-slim

RUN pip install --no-cache-dir numpy protobuf==3.19.4 grpcio grpcio-tools grpcio-reflection 

WORKDIR /app
COPY faasfunc_server.py faasfunc_pb2.py faasfunc_pb2_grpc.py ./

ENV PORT 50051
CMD python faasfunc_server.py -p $PORT