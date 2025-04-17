package helpers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
)

func TestCELSelector(t *testing.T) {
	env, err := NewEnv(nil)
	assert.NoError(t, err)
	assert.NotNil(t, env)

	tests := []struct {
		name               string
		expressions        []string
		cluster            *clusterapiv1.ManagedCluster
		expectedMatch      bool
		expectedCost       int64
		expectCompileError bool
	}{
		{
			name: "valid expression matches",
			expressions: []string{
				`managedCluster.metadata.labels["env"] == "prod"`,
			},
			cluster: &clusterapiv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"env": "prod"},
				},
			},
			expectedMatch: true,
			expectedCost:  5,
		},
		{
			name: "valid expression no match",
			expressions: []string{
				`managedCluster.metadata.labels["env"] == "prod"`,
			},
			cluster: &clusterapiv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"env": "dev"},
				},
			},
			expectedMatch: false,
			expectedCost:  5,
		},
		{
			name: "invalid expression",
			expressions: []string{
				`invalid.expression`,
			},
			cluster:            &clusterapiv1.ManagedCluster{},
			expectCompileError: true,
			expectedMatch:      false,
			expectedCost:       5,
		},
		{
			name: "multiple expressions all match",
			expressions: []string{
				`managedCluster.metadata.labels["env"] == "prod"`,
				`managedCluster.metadata.labels["region"] == "us-east-1"`,
			},
			cluster: &clusterapiv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"env":    "prod",
						"region": "us-east-1",
					},
				},
			},
			expectedMatch: true,
			expectedCost:  10,
		},
		{
			name: "multiple expressions one fails",
			expressions: []string{
				`managedCluster.metadata.labels["env"] == "prod"`,
				`managedCluster.metadata.labels["region"] == "us-east-1"`,
			},
			cluster: &clusterapiv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"env":    "prod",
						"region": "us-west-1",
					},
				},
			},
			expectedMatch: false,
			expectedCost:  10,
		},
		{
			name: "multiple expressions running out of cost budget",
			expressions: []string{
				`managedCluster.metadata.labels["env"] == "prod"`,
				`managedCluster.metadata.labels["env"] == "prod"`,
				`managedCluster.metadata.labels["region"] == "us-east-1"`,
			},
			cluster: &clusterapiv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"env":    "prod",
						"region": "us-east-1",
					},
				},
			},
			expectedMatch: false,
			expectedCost:  11,
		},
		{
			name: "nil cluster",
			expressions: []string{
				`managedCluster.metadata.labels["env"] == "prod"`,
			},
			cluster:            nil,
			expectedMatch:      false,
			expectCompileError: false,
			expectedCost:       3,
		},
		{
			name:               "empty expressions",
			expressions:        []string{},
			cluster:            &clusterapiv1.ManagedCluster{},
			expectedMatch:      true,
			expectCompileError: false,
			expectedCost:       0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selector := NewCELSelector(env, test.expressions, nil)
			results := selector.Compile()

			if test.expectCompileError {
				for _, result := range results {
					assert.NotNil(t, result.Error)
				}
				return
			}

			for _, result := range results {
				assert.Nil(t, result.Error)
			}

			isValid, remainingBudget := selector.Validate(context.TODO(), test.cluster, 10)
			assert.Equal(t, test.expectedMatch, isValid)
			assert.Equal(t, test.expectedCost, 10-remainingBudget)
		})
	}
}
