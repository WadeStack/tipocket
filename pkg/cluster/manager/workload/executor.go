package workload

import (
	"bytes"
	"fmt"

	"github.com/juju/errors"
	"go.uber.org/zap"

	"github.com/pingcap/tipocket/pkg/cluster/manager/artifacts"
	"github.com/pingcap/tipocket/pkg/cluster/manager/types"
	"github.com/pingcap/tipocket/pkg/cluster/manager/util"
)

const brImage = "pingcap/br:nightly"

// RunWorkload creates the workload docker container and injects necessary
// environment variables
func RunWorkload(
	cr *types.ClusterRequest,
	resources []*types.Resource,
	rris []*types.ResourceRequestItem,
	wr *types.WorkloadRequest,
	artifactUUID string,
	envs map[string]string,
) (dockerExecutor *util.DockerExecutor, containerID string, out *bytes.Buffer, err error) {
	rriItemID2Resource, component2Resources := types.BuildClusterMap(resources, rris)
	resource := rriItemID2Resource[wr.RRIItemID]
	host := resource.IP
	if envs == nil {
		envs = make(map[string]string)
	}
	var (
		rs *types.Resource
	)
	envs["CLUSTER_ID"] = fmt.Sprintf("%d", cr.ID)
	envs["CLUSTER_NAME"] = cr.Name
	envs["API_SERVER"] = fmt.Sprintf("http://%s", util.Addr)
	envs["ARTIFACT_URL"] = fmt.Sprintf("%s/%s/%s", util.S3Endpoint, artifacts.ArtifactDownloadPath(cr.ID), artifactUUID)

	if rs, err = util.RandomResource(component2Resources["pd"]); err != nil {
		return nil, "", nil, errors.Trace(err)
	}
	envs["PD_ADDR"] = fmt.Sprintf("%s:2379", rs.IP)

	if len(component2Resources["tidb"]) != 0 {
		if rs, err = util.RandomResource(component2Resources["tidb"]); err != nil {
			return nil, "", nil, errors.Trace(err)
		}
		envs["TIDB_ADDR"] = fmt.Sprintf("%s:4000", rs.IP)
	}

	if rs, err = util.RandomResource(component2Resources["prometheus"]); err != nil {
		return nil, "", nil, errors.Trace(err)
	}
	envs["PROM_ADDR"] = fmt.Sprintf("http://%s:9090", rs.IP)

	dockerExecutor, err = util.NewDockerExecutor(fmt.Sprintf("tcp://%s:2375", host))
	if err != nil {
		return nil, "", nil, errors.Trace(err)
	}
	containerID, out, err = dockerExecutor.Run(wr.DockerImage, envs, wr.Cmd, wr.Args...)
	return
}

// RestoreData uses BR to restore backup data from S3 storage
func RestoreData(restorePath string, pdHost string, host string) (*bytes.Buffer, error) {
	dockerExecutor, err := util.NewDockerExecutor(fmt.Sprintf("tcp://%s:2375", host))
	if err != nil {
		return nil, err
	}
	envs := make(map[string]string)
	envs["PD_ADDR"] = fmt.Sprintf("%s:2379", pdHost)
	envs["S3_ENDPOINT"] = fmt.Sprintf("http://%s", util.S3Endpoint)
	envs["AWS_ACCESS_KEY_ID"] = util.AwsAccessKeyID
	envs["AWS_SECRET_ACCESS_KEY"] = util.AwsSecretAccessKey
	containerID, o, err := dockerExecutor.Run(brImage,
		envs,
		&[]string{"/bin/bash"}[0],
		"-c", fmt.Sprintf("/br restore full --pd $PD_ADDR --storage s3://"+restorePath+" --s3.endpoint $S3_ENDPOINT --send-credentials-to-tikv=true"))
	if containerID != "" {
		defer func() {
			err := dockerExecutor.RmContainer(containerID)
			if err != nil {
				zap.L().Error("rm container failed", zap.String("container id", containerID), zap.Error(err))
			}
		}()
	}
	return o, err
}

// BackupData uses BR to backup data to S3 storage
func BackupData(backupPath string, pdHost string, host string) (*bytes.Buffer, error) {
	dockerExecutor, err := util.NewDockerExecutor(fmt.Sprintf("tcp://%s:2375", host))
	if err != nil {
		return nil, err
	}
	envs := make(map[string]string)
	envs["PD_ADDR"] = fmt.Sprintf("%s:2379", pdHost)
	envs["S3_ENDPOINT"] = fmt.Sprintf("http://%s", util.S3Endpoint)
	envs["AWS_ACCESS_KEY_ID"] = util.AwsAccessKeyID
	envs["AWS_SECRET_ACCESS_KEY"] = util.AwsSecretAccessKey
	containerID, o, err := dockerExecutor.Run(brImage,
		envs,
		&[]string{"/bin/bash"}[0],
		"-c", fmt.Sprintf("/br backup full --pd $PD_ADDR --storage s3://"+backupPath+" --s3.endpoint $S3_ENDPOINT --send-credentials-to-tikv=true"))
	if containerID != "" {
		defer func() {
			err := dockerExecutor.RmContainer(containerID)
			if err != nil {
				zap.L().Error("rm container failed", zap.String("container id", containerID), zap.Error(err))
			}
		}()
	}
	return o, err
}
