This is an upgrade test case for Clustered deployment (s1 - standalone)
1. install splunk operator 1.0.5 to specific namespace
2. make sure the installation is complete and all the CRD are installed along with deployment
3. create S1 deployment  custom resources - standalone custom resource
4. check S1 deployment instanances are up and running
5. upgrade splunk operator to 1.1.0 using upgrade script - step 04
6. check the old opeartor is cleaned up and latest operator is installed in splunk-operator namespace
7. check S1 deployment instanances are up and running
8. Cleanup standalone s1 instanances
9. Cleanup splunk operator 1.1.0