N=100
if [ "$#" -gt 0 ]; then
    N=$1
fi
for i in `seq 1 $N`
do
    go test -v .
    if [ $? -ne 0 ]; then
        echo "Error occurred at ${i}th test."
        exit 1
    fi
done
echo "Test succeeded $i times."
