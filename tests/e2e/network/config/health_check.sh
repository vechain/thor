#!/bin/sh

# Set the URL
url="http://localhost:8669/blocks/best"

# Make the HTTP request and store the response in a variable
response=$(wget -qO- $url)

echo $response

# Extract the value of "number" from the JSON response using grep
number=$(echo $response | grep -o '"number":[^,}]*' | awk -F: '{print $2}' | tr -d '[:space:]')

# Check if the number is greater than 0
if [ $number -gt 0 ]; then
  echo "Health check passed! Number is greater than 0."
  exit 0
else
  echo "Health check failed! Unexpected response or number is not greater than 0:"
  echo $response
  exit 1
fi
