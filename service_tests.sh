
echo "executing go test (i show only end result of test execution)"
go test ./...

echo "executing go test -v (i show details of each test cases)"
go test ./...  -v

echo "executing go test -cover (i show overall test coverage)"
go test  ./... -cover

echo "executing go test -coverprofile (i output the coverage profile to testcoverage.out  which is non human readable)"
go test  ./... -coverprofile=testcoverage.out


echo "executing go tool cover -html=testcoverage (i use testcoverage.out file to generate human readable html files ,which can be used to detect which lines are covered)"
go tool cover -html=testcoverage.out