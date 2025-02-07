package api_test

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/jt-nti/gproto"
	v1alpha "github.com/kaskada-ai/kaskada/gen/proto/go/kaskada/kaskada/v1alpha"
	helpers "github.com/kaskada-ai/kaskada/tests/integration/shared/helpers"
	. "github.com/kaskada-ai/kaskada/tests/integration/shared/matchers"
)

var _ = Describe("Query V1 gRPC with dataTokens", Ordered, func() {
	var (
		ctx                context.Context
		cancel             context.CancelFunc
		conn               *grpc.ClientConn
		tableClient        v1alpha.TableServiceClient
		queryClient        v1alpha.QueryServiceClient
		table              *v1alpha.Table
		createQueryRequest *v1alpha.CreateQueryRequest
		firstDataTokenId   string
		secondDataTokenId  string
		firstResults       []interface{}
		secondResults      []interface{}
		tableName          string
	)

	BeforeAll(func() {
		//get connection to wren
		ctx, cancel, conn = grpcConfig.GetContextCancelConnection(10)
		ctx = metadata.AppendToOutgoingContext(ctx, "client-id", *integrationClientID)

		// get a grpc client for the table & compute services
		tableClient = v1alpha.NewTableServiceClient(conn)
		queryClient = v1alpha.NewQueryServiceClient(conn)

		tableName = "query_v1_test"

		// create a table, load first file
		table = &v1alpha.Table{
			TableName:           tableName,
			TimeColumnName:      "purchase_time",
			EntityKeyColumnName: "customer_id",
			SubsortColumnName: &wrapperspb.StringValue{
				Value: "subsort_id",
			},
		}
		_, err := tableClient.CreateTable(ctx, &v1alpha.CreateTableRequest{Table: table})
		Expect(err).ShouldNot(HaveOccurredGrpc())

		res := helpers.LoadTestFileIntoTable(ctx, conn, table, "purchases/purchases_part1.parquet")
		Expect(res.DataTokenId).ShouldNot(BeEmpty())
		firstDataTokenId = res.DataTokenId

		// define a query to run on the table
		destination := &v1alpha.Destination{}
		destination.Destination = &v1alpha.Destination_ObjectStore{
			ObjectStore: &v1alpha.ObjectStoreDestination{
				FileType: v1alpha.FileType_FILE_TYPE_PARQUET,
			},
		}
		query := &v1alpha.Query{
			Expression: `
{
time: query_v1_test.purchase_time,
entity: query_v1_test.customer_id,
max_amount: query_v1_test.amount | max(),
min_amount: query_v1_test.amount | min(),
}`,
			Destination:    destination,
			ResultBehavior: v1alpha.Query_RESULT_BEHAVIOR_ALL_RESULTS,
		}
		queryOptions := &v1alpha.QueryOptions{
			ExperimentalFeatures: true,
			PresignResults:       true,
		}
		createQueryRequest = &v1alpha.CreateQueryRequest{
			Query:        query,
			QueryOptions: queryOptions,
		}
	})

	AfterAll(func() {
		// clean up items used in the test
		_, err := tableClient.DeleteTable(ctx, &v1alpha.DeleteTableRequest{TableName: tableName})
		Expect(err).ShouldNot(HaveOccurredGrpc())

		cancel()
		conn.Close()
	})

	Context("When the table schema is created correctly", Ordered, func() {
		Describe("Run the streaming query using dry-run", func() {
			It("should return a single response with schema info, but no results", func() {
				createQueryRequest.QueryOptions.DryRun = true
				stream, err := queryClient.CreateQuery(ctx, createQueryRequest)
				createQueryRequest.QueryOptions.DryRun = false
				Expect(err).ShouldNot(HaveOccurredGrpc())
				Expect(stream).ShouldNot(BeNil())

				queryResponses, err := helpers.GetCreateQueryResponses(stream)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(len(queryResponses)).Should(Equal(1))

				firstResponse := queryResponses[0]

				Expect(firstResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_ANALYSIS))
				VerifyRequestDetails(firstResponse.RequestDetails)
				Expect(firstResponse.Config.DataTokenId).ShouldNot(BeEmpty())
				Expect(firstResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths()).Should(BeNil())
				Expect(firstResponse.Analysis.Schema.GetFields()).Should(ContainElements(
					gproto.Equal(primitiveSchemaField("time", v1alpha.DataType_PRIMITIVE_TYPE_TIMESTAMP_NANOSECOND)),
					gproto.Equal(primitiveSchemaField("entity", v1alpha.DataType_PRIMITIVE_TYPE_STRING)),
					gproto.Equal(primitiveSchemaField("max_amount", v1alpha.DataType_PRIMITIVE_TYPE_I64)),
					gproto.Equal(primitiveSchemaField("min_amount", v1alpha.DataType_PRIMITIVE_TYPE_I64)),
				))
			})
		})

		Describe("Run the streaming query without specifying a dataToken", func() {
			It("should return at least 3 responses, with results from the first file", func() {
				stream, err := queryClient.CreateQuery(ctx, createQueryRequest)
				Expect(err).ShouldNot(HaveOccurredGrpc())
				Expect(stream).ShouldNot(BeNil())

				queryResponses, err := helpers.GetCreateQueryResponses(stream)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(len(queryResponses)).Should(BeNumerically(">=", 3))

				var firstResponse, secondResponse, thirdResponse, lastResponse *v1alpha.CreateQueryResponse

				firstResponse, queryResponses = queryResponses[0], queryResponses[1:]
				secondResponse, queryResponses = queryResponses[0], queryResponses[1:]
				thirdResponse, queryResponses = queryResponses[0], queryResponses[1:]

				Expect(firstResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_ANALYSIS))
				VerifyRequestDetails(firstResponse.RequestDetails)
				Expect(firstResponse.Config.DataTokenId).Should(Equal(firstDataTokenId))
				Expect(firstResponse.Analysis.Schema.GetFields()).Should(ContainElements(
					gproto.Equal(primitiveSchemaField("time", v1alpha.DataType_PRIMITIVE_TYPE_TIMESTAMP_NANOSECOND)),
					gproto.Equal(primitiveSchemaField("entity", v1alpha.DataType_PRIMITIVE_TYPE_STRING)),
					gproto.Equal(primitiveSchemaField("max_amount", v1alpha.DataType_PRIMITIVE_TYPE_I64)),
					gproto.Equal(primitiveSchemaField("min_amount", v1alpha.DataType_PRIMITIVE_TYPE_I64)),
				))
				_, err = uuid.Parse(secondResponse.QueryId)
				Expect(err).Should(BeNil())
				Expect(thirdResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_PREPARING))

				lastResponse, queryResponses = queryResponses[len(queryResponses)-1], queryResponses[:len(queryResponses)-1]
				Expect(lastResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_SUCCESS))
				Expect(lastResponse.RequestDetails.RequestId).Should(Equal(firstResponse.RequestDetails.RequestId))
				Expect(lastResponse.Metrics).ShouldNot(BeNil())
				Expect(lastResponse.Metrics.OutputFiles).Should(BeEquivalentTo(1))
				Expect(lastResponse.Metrics.TotalInputRows).Should(BeEquivalentTo(10))
				Expect(lastResponse.Metrics.ProcessedInputRows).Should(BeEquivalentTo(10))
				Expect(lastResponse.Metrics.ProducedOutputRows).Should(BeEquivalentTo(10))

				resultUrls := []string{}
				for _, queryResponse := range queryResponses {
					Expect(queryResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_COMPUTING))
					Expect(queryResponse.RequestDetails.RequestId).Should(Equal(firstResponse.RequestDetails.RequestId))

					if queryResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths() != nil {
						resultUrls = append(resultUrls, queryResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths()...)
					}
				}

				Expect(len(resultUrls)).Should(Equal(1))
				firstResults = helpers.DownloadParquet(resultUrls[0])

				Expect(firstResults).Should(HaveLen(10))
				Expect(firstResults[9]).Should(MatchFields(IgnoreExtras, Fields{
					"Time":       PointTo(BeEquivalentTo(1578182400000000000)),
					"Entity":     PointTo(Equal("patrick")),
					"Max_amount": PointTo(BeEquivalentTo(5000)),
					"Min_amount": PointTo(BeEquivalentTo(3)),
				}))
			})
		})

		Describe("Load the second file into the table, copying the file to the kaskada filesystem", func() {
			It("Should work without error and return a dataToken", func() {
				res, err := tableClient.LoadData(ctx, &v1alpha.LoadDataRequest{
					TableName: table.TableName,
					SourceData: &v1alpha.LoadDataRequest_FileInput{
						FileInput: &v1alpha.FileInput{
							FileType: v1alpha.FileType_FILE_TYPE_PARQUET,
							Uri:      helpers.GetFileURI("purchases/purchases_part2.parquet"),
						},
					},
					CopyToFilesystem: true,
				})
				Expect(err).ShouldNot(HaveOccurredGrpc())
				Expect(res).ShouldNot(BeNil())

				Expect(res.DataTokenId).ShouldNot(BeEmpty())
				secondDataTokenId = res.DataTokenId
				Expect(secondDataTokenId).ShouldNot(Equal(firstDataTokenId))
			})
		})

		Describe("Run the query again without specifying a dataToken", func() {
			It("should return at least 3 responses, with results from both files", func() {
				stream, err := queryClient.CreateQuery(ctx, createQueryRequest)
				Expect(err).ShouldNot(HaveOccurredGrpc())
				Expect(stream).ShouldNot(BeNil())

				queryResponses, err := helpers.GetCreateQueryResponses(stream)
				Expect(err).ShouldNot(HaveOccurredGrpc())
				Expect(len(queryResponses)).Should(BeNumerically(">=", 3))

				var firstResponse, secondResponse, thirdResponse, lastResponse *v1alpha.CreateQueryResponse

				firstResponse, queryResponses = queryResponses[0], queryResponses[1:]
				secondResponse, queryResponses = queryResponses[0], queryResponses[1:]
				thirdResponse, queryResponses = queryResponses[0], queryResponses[1:]

				Expect(firstResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_ANALYSIS))
				VerifyRequestDetails(firstResponse.RequestDetails)
				Expect(firstResponse.Config.DataTokenId).Should(Equal(secondDataTokenId))
				_, err = uuid.Parse(secondResponse.QueryId)
				Expect(err).Should(BeNil())
				Expect(thirdResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_PREPARING))

				lastResponse, queryResponses = queryResponses[len(queryResponses)-1], queryResponses[:len(queryResponses)-1]
				Expect(lastResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_SUCCESS))
				Expect(lastResponse.RequestDetails.RequestId).Should(Equal(firstResponse.RequestDetails.RequestId))
				Expect(lastResponse.Metrics).ShouldNot(BeNil())
				Expect(lastResponse.Metrics.OutputFiles).Should(BeEquivalentTo(1))
				Expect(lastResponse.Metrics.TotalInputRows).Should(BeEquivalentTo(15))
				Expect(lastResponse.Metrics.ProcessedInputRows).Should(BeEquivalentTo(15))
				Expect(lastResponse.Metrics.ProducedOutputRows).Should(BeEquivalentTo(15))

				resultUrls := []string{}
				for _, queryResponse := range queryResponses {
					Expect(queryResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_COMPUTING))
					Expect(queryResponse.RequestDetails.RequestId).Should(Equal(firstResponse.RequestDetails.RequestId))

					if queryResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths() != nil {
						resultUrls = append(resultUrls, queryResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths()...)
					}
				}

				Expect(len(resultUrls)).Should(Equal(1))
				secondResults = helpers.DownloadParquet(resultUrls[0])

				Expect(secondResults).Should(HaveLen(15))
				Expect(secondResults[9]).Should(MatchFields(IgnoreExtras, Fields{
					"Time":       PointTo(BeEquivalentTo(1578182400000000000)),
					"Entity":     PointTo(Equal("patrick")),
					"Max_amount": PointTo(BeEquivalentTo(5000)),
					"Min_amount": PointTo(BeEquivalentTo(3)),
				}))
				Expect(secondResults[13]).Should(MatchFields(IgnoreExtras, Fields{
					"Time":       PointTo(BeEquivalentTo(1578441600000000000)),
					"Entity":     PointTo(Equal("karen")),
					"Max_amount": PointTo(BeEquivalentTo(9)),
					"Min_amount": PointTo(BeEquivalentTo(2)),
				}))
			})
		})

		Describe("Run the query again, specifying the first dataToken", func() {
			It("should return at least 3 responses, with results that match the first results", func() {
				createQueryRequest.Query.DataTokenId = &wrapperspb.StringValue{
					Value: firstDataTokenId,
				}

				stream, err := queryClient.CreateQuery(ctx, createQueryRequest)
				Expect(err).ShouldNot(HaveOccurredGrpc())
				Expect(stream).ShouldNot(BeNil())

				queryResponses, err := helpers.GetCreateQueryResponses(stream)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(len(queryResponses)).Should(BeNumerically(">=", 3))

				var firstResponse, secondResponse, thirdResponse, lastResponse *v1alpha.CreateQueryResponse

				firstResponse, queryResponses = queryResponses[0], queryResponses[1:]
				secondResponse, queryResponses = queryResponses[0], queryResponses[1:]
				thirdResponse, queryResponses = queryResponses[0], queryResponses[1:]
				Expect(firstResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_ANALYSIS))
				VerifyRequestDetails(firstResponse.RequestDetails)
				Expect(firstResponse.Config.DataTokenId).Should(Equal(firstDataTokenId))
				_, err = uuid.Parse(secondResponse.QueryId)
				Expect(err).Should(BeNil())
				Expect(thirdResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_PREPARING))

				lastResponse, queryResponses = queryResponses[len(queryResponses)-1], queryResponses[:len(queryResponses)-1]
				Expect(lastResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_SUCCESS))
				Expect(lastResponse.RequestDetails.RequestId).Should(Equal(firstResponse.RequestDetails.RequestId))

				resultUrls := []string{}
				for _, queryResponse := range queryResponses {
					Expect(queryResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_COMPUTING))
					Expect(queryResponse.RequestDetails.RequestId).Should(Equal(firstResponse.RequestDetails.RequestId))

					if queryResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths() != nil {
						resultUrls = append(resultUrls, queryResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths()...)
					}
				}

				Expect(len(resultUrls)).Should(Equal(1))
				results := helpers.DownloadParquet(resultUrls[0])
				Expect(results).Should(BeEquivalentTo(firstResults))
			})
		})

		Describe("Run the query again, specifying the second dataToken", func() {
			It("should return at least 3 responses, with results that match the second results", func() {
				createQueryRequest.Query.DataTokenId = &wrapperspb.StringValue{
					Value: secondDataTokenId,
				}
				stream, err := queryClient.CreateQuery(ctx, createQueryRequest)
				Expect(err).ShouldNot(HaveOccurredGrpc())
				Expect(stream).ShouldNot(BeNil())

				queryResponses, err := helpers.GetCreateQueryResponses(stream)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(len(queryResponses)).Should(BeNumerically(">=", 3))

				var firstResponse, secondResponse, thirdResponse, lastResponse *v1alpha.CreateQueryResponse

				firstResponse, queryResponses = queryResponses[0], queryResponses[1:]
				secondResponse, queryResponses = queryResponses[0], queryResponses[1:]
				thirdResponse, queryResponses = queryResponses[0], queryResponses[1:]
				Expect(firstResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_ANALYSIS))
				VerifyRequestDetails(firstResponse.RequestDetails)
				Expect(firstResponse.Config.DataTokenId).Should(Equal(secondDataTokenId))
				_, err = uuid.Parse(secondResponse.QueryId)
				Expect(err).Should(BeNil())
				Expect(thirdResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_PREPARING))

				lastResponse, queryResponses = queryResponses[len(queryResponses)-1], queryResponses[:len(queryResponses)-1]
				Expect(lastResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_SUCCESS))
				Expect(lastResponse.RequestDetails.RequestId).Should(Equal(firstResponse.RequestDetails.RequestId))

				resultUrls := []string{}
				for _, queryResponse := range queryResponses {
					Expect(queryResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_COMPUTING))
					Expect(queryResponse.RequestDetails.RequestId).Should(Equal(firstResponse.RequestDetails.RequestId))

					if queryResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths() != nil {
						resultUrls = append(resultUrls, queryResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths()...)
					}
				}

				Expect(len(resultUrls)).Should(Equal(1))
				results := helpers.DownloadParquet(resultUrls[0])
				Expect(results).Should(BeEquivalentTo(secondResults))
			})
		})

		Describe("Run the query again, using results-behavior final-results", func() {
			It("should return at least 3 responses, with only the final results for each entity", func() {
				createQueryRequest.Query.ResultBehavior = v1alpha.Query_RESULT_BEHAVIOR_FINAL_RESULTS

				stream, err := queryClient.CreateQuery(ctx, createQueryRequest)
				Expect(err).ShouldNot(HaveOccurredGrpc())
				Expect(stream).ShouldNot(BeNil())

				queryResponses, err := helpers.GetCreateQueryResponses(stream)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(len(queryResponses)).Should(BeNumerically(">=", 3))

				var firstResponse, secondResponse, thirdResponse, lastResponse *v1alpha.CreateQueryResponse

				firstResponse, queryResponses = queryResponses[0], queryResponses[1:]
				secondResponse, queryResponses = queryResponses[0], queryResponses[1:]
				thirdResponse, queryResponses = queryResponses[0], queryResponses[1:]
				Expect(firstResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_ANALYSIS))
				VerifyRequestDetails(firstResponse.RequestDetails)
				Expect(firstResponse.Config.DataTokenId).Should(Equal(secondDataTokenId))
				_, err = uuid.Parse(secondResponse.QueryId)
				Expect(err).Should(BeNil())
				Expect(thirdResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_PREPARING))

				lastResponse, queryResponses = queryResponses[len(queryResponses)-1], queryResponses[:len(queryResponses)-1]
				Expect(lastResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_SUCCESS))
				Expect(lastResponse.RequestDetails.RequestId).Should(Equal(firstResponse.RequestDetails.RequestId))
				Expect(lastResponse.Metrics).ShouldNot(BeNil())
				Expect(lastResponse.Metrics.OutputFiles).Should(BeEquivalentTo(1))
				Expect(lastResponse.Metrics.TotalInputRows).Should(BeEquivalentTo(15))
				Expect(lastResponse.Metrics.ProcessedInputRows).Should(BeEquivalentTo(15))
				Expect(lastResponse.Metrics.ProducedOutputRows).Should(BeEquivalentTo(3))

				resultUrls := []string{}
				for _, queryResponse := range queryResponses {
					Expect(queryResponse.State).Should(Equal(v1alpha.CreateQueryResponse_STATE_COMPUTING))
					Expect(queryResponse.RequestDetails.RequestId).Should(Equal(firstResponse.RequestDetails.RequestId))

					if queryResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths() != nil {
						resultUrls = append(resultUrls, queryResponse.GetDestination().GetObjectStore().GetOutputPaths().GetPaths()...)
					}
				}

				Expect(len(resultUrls)).Should(Equal(1))
				results := helpers.DownloadParquet(resultUrls[0])
				Expect(results).Should(HaveLen(3))
				Expect(results).Should(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Time":       PointTo(BeEquivalentTo(1578441600000000000)),
						"Entity":     PointTo(Equal("patrick")),
						"Max_amount": PointTo(BeEquivalentTo(5000)),
						"Min_amount": PointTo(BeEquivalentTo(2)),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Time":       PointTo(BeEquivalentTo(1578441600000000000)),
						"Entity":     PointTo(Equal("karen")),
						"Max_amount": PointTo(BeEquivalentTo(9)),
						"Min_amount": PointTo(BeEquivalentTo(2)),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Time":       PointTo(BeEquivalentTo(1578355200000000000)),
						"Entity":     PointTo(Equal("spongebob")),
						"Max_amount": PointTo(BeEquivalentTo(34)),
						"Min_amount": PointTo(BeEquivalentTo(7)),
					}),
				))
			})
		})
	})
})
