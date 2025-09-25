<?php
namespace SteemDB\Tasks;

use Phalcon\Cli\Task;

class DistributionTask extends Task
{
    // default action if no action gived in command
    public function mainAction()
    {
        $this->updateAction();
    }

    public function updateAction()
    {
        $this->logger->info('Start update distribution.');
        $props = $this->steemd->getProps();
        $startTime = microtime(true);
        $this->util->updateDistribution($props);
        $endTime = microtime(true);
        $costTime = ($endTime - $startTime) * 1000;
        $this->logger->debug("distribution() tasks {$costTime} ms");
        $this->logger->info('End update distribution.');
    }
}