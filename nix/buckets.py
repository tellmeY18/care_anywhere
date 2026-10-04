"""Native equivalent of CARE Clinic's deployments/minio/entrypoint.sh bootstrap."""
import os
import json
import time
import boto3

s3 = boto3.client("s3", endpoint_url=os.environ["BUCKET_ENDPOINT"],
                  aws_access_key_id=os.environ["BUCKET_KEY"],
                  aws_secret_access_key=os.environ["BUCKET_SECRET"],
                  region_name=os.environ["BUCKET_REGION"])
for attempt in range(30):
    try:
        existing = {b["Name"] for b in s3.list_buckets()["Buckets"]}
        break
    except Exception:
        if attempt == 29:
            raise
        time.sleep(2)
for bucket in [os.environ["FILE_UPLOAD_BUCKET"], os.environ["FACILITY_S3_BUCKET"]]:
    if bucket not in existing:
        s3.create_bucket(Bucket=bucket)
bucket = os.environ["FACILITY_S3_BUCKET"]
s3.put_bucket_policy(Bucket=bucket, Policy=json.dumps({"Version": "2012-10-17", "Statement": [
    {"Effect": "Allow", "Principal": "*", "Action": "s3:GetObject", "Resource": f"arn:aws:s3:::{bucket}/*"}
]}))
