# Motivation 1
Here are the requirments to run the motivation 1 experiment.
- Using `config/contaienrd.toml` for containerd and add service monitor to scrape the metric
- Put invocation seqences of each experiment at `~/traces` 
- Create results folders of each epxeriment at `~/results`
- Run `cd ~/loader && ./run_motivation_1.sh` 
## Plotting
Run `python3 ~/loader/plotter/motivation_1/plot.py`
