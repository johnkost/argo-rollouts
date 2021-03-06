package rollout

import (
	"fmt"
	"testing"

	"github.com/argoproj/argo-rollouts/rollout/trafficrouting/alb"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/argoproj/argo-rollouts/rollout/trafficrouting/istio"
	"github.com/argoproj/argo-rollouts/rollout/trafficrouting/nginx"
	"github.com/argoproj/argo-rollouts/utils/conditions"
	logutil "github.com/argoproj/argo-rollouts/utils/log"
)

type FakeTrafficRoutingReconciler struct {
	errMessage                 string
	controllerSetDesiredWeight int32
}

func (r *FakeTrafficRoutingReconciler) Reconcile(desiredWeight int32) error {
	if r.errMessage != "" {
		return fmt.Errorf(r.errMessage)
	}
	r.controllerSetDesiredWeight = desiredWeight
	return nil
}

func (r *FakeTrafficRoutingReconciler) Type() string {
	return "fake"
}

func TestReconcileTrafficRoutingReturnErr(t *testing.T) {
	f := newFixture(t)
	defer f.Close()

	steps := []v1alpha1.CanaryStep{
		{
			SetWeight: pointer.Int32Ptr(10),
		},
	}
	r1 := newCanaryRollout("foo", 10, nil, steps, pointer.Int32Ptr(0), intstr.FromInt(1), intstr.FromInt(0))
	r2 := bumpVersion(r1)
	r2.Spec.Strategy.Canary.TrafficRouting = &v1alpha1.RolloutTrafficRouting{}
	f.fakeTrafficRouting.errMessage = "Error message"

	rs1 := newReplicaSetWithStatus(r1, 10, 10)
	rs2 := newReplicaSetWithStatus(r2, 1, 1)

	f.kubeobjects = append(f.kubeobjects, rs1, rs2)
	rs1PodHash := rs1.Labels[v1alpha1.DefaultRolloutUniqueLabelKey]
	f.replicaSetLister = append(f.replicaSetLister, rs1, rs2)

	r2 = updateCanaryRolloutStatus(r2, rs1PodHash, 10, 0, 10, false)
	f.rolloutLister = append(f.rolloutLister, r2)
	f.objects = append(f.objects, r2)

	f.runExpectError(getKey(r2, t), true)

}

func TestRolloutUseDesiredWeight(t *testing.T) {
	f := newFixture(t)
	defer f.Close()

	steps := []v1alpha1.CanaryStep{
		{
			SetWeight: pointer.Int32Ptr(10),
		},
		{
			Pause: &v1alpha1.RolloutPause{},
		},
	}
	r1 := newCanaryRollout("foo", 10, nil, steps, pointer.Int32Ptr(1), intstr.FromInt(1), intstr.FromInt(0))
	r2 := bumpVersion(r1)
	r2.Spec.Strategy.Canary.TrafficRouting = &v1alpha1.RolloutTrafficRouting{}

	progressingCondition, _ := newProgressingCondition(conditions.PausedRolloutReason, r2, "")
	conditions.SetRolloutCondition(&r2.Status, progressingCondition)

	rs1 := newReplicaSetWithStatus(r1, 10, 10)
	rs2 := newReplicaSetWithStatus(r2, 1, 1)

	f.kubeobjects = append(f.kubeobjects, rs1, rs2)
	rs1PodHash := rs1.Labels[v1alpha1.DefaultRolloutUniqueLabelKey]
	f.replicaSetLister = append(f.replicaSetLister, rs1, rs2)

	r2 = updateCanaryRolloutStatus(r2, rs1PodHash, 10, 0, 10, true)
	f.rolloutLister = append(f.rolloutLister, r2)
	f.objects = append(f.objects, r2)

	f.expectPatchRolloutAction(r2)
	f.run(getKey(r2, t))

	assert.Equal(t, int32(10), f.fakeTrafficRouting.controllerSetDesiredWeight)
}

func TestRolloutUsePreviousSetWeight(t *testing.T) {
	f := newFixture(t)
	defer f.Close()

	steps := []v1alpha1.CanaryStep{
		{
			SetWeight: pointer.Int32Ptr(10),
		},
		{
			SetWeight: pointer.Int32Ptr(20),
		},
	}
	r1 := newCanaryRollout("foo", 10, nil, steps, pointer.Int32Ptr(1), intstr.FromInt(1), intstr.FromInt(0))
	r2 := bumpVersion(r1)
	r2.Spec.Strategy.Canary.TrafficRouting = &v1alpha1.RolloutTrafficRouting{}

	rs1 := newReplicaSetWithStatus(r1, 10, 10)
	rs2 := newReplicaSetWithStatus(r2, 1, 1)

	f.kubeobjects = append(f.kubeobjects, rs1, rs2)
	rs1PodHash := rs1.Labels[v1alpha1.DefaultRolloutUniqueLabelKey]
	f.replicaSetLister = append(f.replicaSetLister, rs1, rs2)

	r2 = updateCanaryRolloutStatus(r2, rs1PodHash, 10, 0, 10, false)
	f.rolloutLister = append(f.rolloutLister, r2)
	f.objects = append(f.objects, r2)

	f.expectUpdateReplicaSetAction(rs2)
	f.expectPatchRolloutAction(r2)
	f.run(getKey(r2, t))

	assert.Equal(t, int32(10), f.fakeTrafficRouting.controllerSetDesiredWeight)
}

func TestRolloutSetWeightToZeroWhenFullyRolledOut(t *testing.T) {
	f := newFixture(t)
	defer f.Close()

	steps := []v1alpha1.CanaryStep{
		{
			SetWeight: pointer.Int32Ptr(10),
		},
	}
	r1 := newCanaryRollout("foo", 10, nil, steps, pointer.Int32Ptr(1), intstr.FromInt(1), intstr.FromInt(0))
	r1.Spec.Strategy.Canary.TrafficRouting = &v1alpha1.RolloutTrafficRouting{}
	f.fakeTrafficRouting.controllerSetDesiredWeight = 10

	rs1 := newReplicaSetWithStatus(r1, 10, 10)

	f.kubeobjects = append(f.kubeobjects, rs1)
	rs1PodHash := rs1.Labels[v1alpha1.DefaultRolloutUniqueLabelKey]
	f.replicaSetLister = append(f.replicaSetLister, rs1)

	r1 = updateCanaryRolloutStatus(r1, rs1PodHash, 10, 0, 10, false)
	f.rolloutLister = append(f.rolloutLister, r1)
	f.objects = append(f.objects, r1)

	f.expectPatchRolloutAction(r1)
	f.run(getKey(r1, t))

	assert.Equal(t, int32(0), f.fakeTrafficRouting.controllerSetDesiredWeight)
}

func TestNewTrafficRoutingReconciler(t *testing.T) {
	rc := Controller{}
	steps := []v1alpha1.CanaryStep{
		{
			SetWeight: pointer.Int32Ptr(10),
		},
	}

	{
		r := newCanaryRollout("foo", 10, nil, steps, pointer.Int32Ptr(1), intstr.FromInt(1), intstr.FromInt(0))
		roCtx := &canaryContext{
			rollout: r,
			log:     logutil.WithRollout(r),
		}
		networkReconciler := rc.NewTrafficRoutingReconciler(roCtx)
		assert.Nil(t, networkReconciler)
	}
	{
		r := newCanaryRollout("foo", 10, nil, steps, pointer.Int32Ptr(1), intstr.FromInt(1), intstr.FromInt(0))
		r.Spec.Strategy.Canary.TrafficRouting = &v1alpha1.RolloutTrafficRouting{}
		roCtx := &canaryContext{
			rollout: r,
			log:     logutil.WithRollout(r),
		}
		networkReconciler := rc.NewTrafficRoutingReconciler(roCtx)
		assert.Nil(t, networkReconciler)
	}
	{
		r := newCanaryRollout("foo", 10, nil, steps, pointer.Int32Ptr(1), intstr.FromInt(1), intstr.FromInt(0))
		r.Spec.Strategy.Canary.TrafficRouting = &v1alpha1.RolloutTrafficRouting{
			Istio: &v1alpha1.IstioTrafficRouting{},
		}
		roCtx := &canaryContext{
			rollout: r,
			log:     logutil.WithRollout(r),
		}
		networkReconciler := rc.NewTrafficRoutingReconciler(roCtx)
		assert.NotNil(t, networkReconciler)
		assert.Equal(t, istio.Type, networkReconciler.Type())
	}
	{
		r := newCanaryRollout("foo", 10, nil, steps, pointer.Int32Ptr(1), intstr.FromInt(1), intstr.FromInt(0))
		r.Spec.Strategy.Canary.TrafficRouting = &v1alpha1.RolloutTrafficRouting{
			Nginx: &v1alpha1.NginxTrafficRouting{},
		}
		roCtx := &canaryContext{
			rollout: r,
			log:     logutil.WithRollout(r),
		}
		networkReconciler := rc.NewTrafficRoutingReconciler(roCtx)
		assert.NotNil(t, networkReconciler)
		assert.Equal(t, nginx.Type, networkReconciler.Type())
	}
	{
		r := newCanaryRollout("foo", 10, nil, steps, pointer.Int32Ptr(1), intstr.FromInt(1), intstr.FromInt(0))
		r.Spec.Strategy.Canary.TrafficRouting = &v1alpha1.RolloutTrafficRouting{
			ALB: &v1alpha1.ALBTrafficRouting{},
		}
		roCtx := &canaryContext{
			rollout: r,
			log:     logutil.WithRollout(r),
		}
		networkReconciler := rc.NewTrafficRoutingReconciler(roCtx)
		assert.NotNil(t, networkReconciler)
		assert.Equal(t, alb.Type, networkReconciler.Type())
	}
}
